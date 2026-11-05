package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrReviewAuthorizationConflict = errors.New("deployment review authorization is stale or mismatched")

var provenanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var agentHubIdentityIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,63}$`)

// ReviewDecisionProvenance contains safe identifiers and hashes only. Reviewer
// identities, review contents and bearer credentials stay in AgentHub.
type ReviewDecisionProvenance struct {
	DecisionID    string   `json:"decision_id"`
	TenantID      string   `json:"tenant_id"`
	MachineID     string   `json:"machine_id"`
	TicketID      string   `json:"ticket_id"`
	TicketSHA256  string   `json:"ticket_sha256"`
	PolicySHA256  string   `json:"policy_sha256"`
	PolicyVersion int64    `json:"policy_version"`
	MatchedRules  []string `json:"matched_rules"`
	ActionSHA256  string   `json:"action_sha256"`
}

func (p ReviewDecisionProvenance) Validate() error {
	if !provenanceIDPattern.MatchString(p.DecisionID) || !agentHubIdentityIDPattern.MatchString(p.TenantID) || !agentHubIdentityIDPattern.MatchString(p.MachineID) ||
		!validAgentHubTicketID(p.TicketID) ||
		!validSHA256Value(p.TicketSHA256) || !validSHA256Value(p.PolicySHA256) ||
		p.PolicyVersion < 1 || !validSHA256Value(p.ActionSHA256) || len(p.MatchedRules) == 0 || len(p.MatchedRules) > 64 {
		return errors.New("AgentHub decision provenance is incomplete")
	}
	seen := make(map[string]struct{}, len(p.MatchedRules))
	for _, rule := range p.MatchedRules {
		if !provenanceIDPattern.MatchString(rule) {
			return errors.New("AgentHub matched rule ID is invalid")
		}
		if _, ok := seen[rule]; ok {
			return errors.New("AgentHub matched rule IDs must be unique")
		}
		seen[rule] = struct{}{}
	}
	return nil
}

// RecordReviewDecision appends a tenant-fenced, generation-fenced authorization
// result only while the exact submitted review remains current. This records
// audit provenance; callers must reauthorize immediately before provider writes.
func (s *Store) RecordReviewDecision(ctx context.Context, tenantID, deploymentID string, generation int64,
	actor, idempotencyKey string, allowed bool, provenance ReviewDecisionProvenance, failureCode string) (DeploymentEvent, error) {
	return s.recordReviewDecision(ctx, tenantID, deploymentID, generation, actor, idempotencyKey,
		allowed, provenance, failureCode, "", "")
}

// RecordReviewDecisionWithLease records worker authorization provenance only
// while the caller owns the live deployment reconcile lease. The lease row,
// desired generation, review, and append-only event are checked in one
// transaction so a reclaimed worker cannot publish a stale decision.
func (s *Store) RecordReviewDecisionWithLease(ctx context.Context, tenantID, deploymentID string, generation int64,
	actor, idempotencyKey, jobID, leaseOwner string, allowed bool, provenance ReviewDecisionProvenance,
	failureCode string) (DeploymentEvent, error) {
	if !validUUID(jobID) || strings.TrimSpace(leaseOwner) == "" || len(leaseOwner) > 255 {
		return DeploymentEvent{}, ErrInvalidRequest
	}
	return s.recordReviewDecision(ctx, tenantID, deploymentID, generation, actor, idempotencyKey,
		allowed, provenance, failureCode, jobID, leaseOwner)
}

func (s *Store) recordReviewDecision(ctx context.Context, tenantID, deploymentID string, generation int64,
	actor, idempotencyKey string, allowed bool, provenance ReviewDecisionProvenance, failureCode,
	jobID, leaseOwner string) (DeploymentEvent, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(idempotencyKey) || generation < 1 || !validReviewActor(actor) {
		return DeploymentEvent{}, ErrInvalidRequest
	}
	if allowed {
		if failureCode != "" || provenance.Validate() != nil ||
			provenance.ActionSHA256 == "" || provenance.TicketID == "" {
			return DeploymentEvent{}, ErrInvalidRequest
		}
	} else if !validFailureCode(failureCode) || failureCode == "" {
		return DeploymentEvent{}, ErrInvalidRequest
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeploymentEvent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if jobID != "" {
		var owned string
		err = tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE id=$1 AND kind='deployment.reconcile'
			AND status='running' AND lease_owner=$2 AND lease_until>now() FOR UPDATE`, jobID, leaseOwner).Scan(&owned)
		if errors.Is(err, pgx.ErrNoRows) {
			return DeploymentEvent{}, ErrLostJobLease
		}
		if err != nil {
			return DeploymentEvent{}, err
		}
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		return DeploymentEvent{}, err
	}
	var current int64
	if err := tx.QueryRow(ctx, `SELECT desired_generation FROM shippin_deployments WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, deploymentID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DeploymentEvent{}, ErrNotFound
		}
		return DeploymentEvent{}, err
	}
	if current != generation {
		return DeploymentEvent{}, ErrGenerationConflict
	}
	var actionSHA, ticketID, state, ticketSHA string
	if err := tx.QueryRow(ctx, `SELECT action_sha256,agenthub_ticket_id,state,coalesce(ticket_sha256,'')
		FROM shippin_deployment_reviews WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3 FOR UPDATE`,
		tenantID, deploymentID, generation).Scan(&actionSHA, &ticketID, &state, &ticketSHA); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DeploymentEvent{}, ErrNotFound
		}
		return DeploymentEvent{}, err
	}
	if state != "pending_review" {
		return DeploymentEvent{}, ErrReviewAuthorizationConflict
	}
	if allowed && (provenance.ActionSHA256 != actionSHA || provenance.TicketID != ticketID || provenance.TicketSHA256 != ticketSHA) {
		return DeploymentEvent{}, ErrReviewAuthorizationConflict
	}

	var prior DeploymentEvent
	var priorRaw []byte
	err = tx.QueryRow(ctx, `SELECT id::text,deployment_id::text,tenant_id::text,generation,event_type,coalesce(failure_code,''),actor,recorded_at,review_provenance
		FROM shippin_deployment_events WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(
		&prior.ID, &prior.DeploymentID, &prior.TenantID, &prior.Generation, &prior.Type, &prior.FailureCode, &prior.Actor, &prior.RecordedAt, &priorRaw)
	if err == nil {
		if prior.DeploymentID != deploymentID || prior.Generation != generation {
			return DeploymentEvent{}, ErrIdempotencyConflict
		}
		if allowed && prior.Type == EventReviewAllowed {
			var stored ReviewDecisionProvenance
			if json.Unmarshal(priorRaw, &stored) == nil {
				if !sameAuthorization(stored, provenance) {
					return DeploymentEvent{}, ErrIdempotencyConflict
				}
				if err := tx.Commit(ctx); err != nil {
					return DeploymentEvent{}, err
				}
				prior.ReviewProvenance = &stored
				return prior, nil
			}
		}
		if !allowed && prior.Type == EventReviewDenied && prior.FailureCode == failureCode {
			if err := tx.Commit(ctx); err != nil {
				return DeploymentEvent{}, err
			}
			return prior, nil
		}
		return DeploymentEvent{}, ErrIdempotencyConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DeploymentEvent{}, err
	}

	eventID, err := uuid.NewV7()
	if err != nil {
		return DeploymentEvent{}, err
	}
	eventType := EventReviewDenied
	var provenanceJSON []byte
	if allowed {
		eventType = EventReviewAllowed
		provenanceJSON, err = json.Marshal(provenance)
		if err != nil {
			return DeploymentEvent{}, err
		}
	}
	var recordedAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO shippin_deployment_events
		(id,deployment_id,tenant_id,generation,event_type,failure_code,actor,review_provenance,idempotency_key)
		VALUES($1,$2,$3,$4,$5,nullif($6,''),$7,$8,$9) RETURNING recorded_at`, eventID, deploymentID, tenantID, generation, eventType,
		failureCode, actor, provenanceJSON, idempotencyKey).Scan(&recordedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return DeploymentEvent{}, ErrIdempotencyConflict
		}
		return DeploymentEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeploymentEvent{}, err
	}
	event := DeploymentEvent{ID: eventID.String(), DeploymentID: deploymentID, TenantID: tenantID,
		Generation: generation, Type: eventType, FailureCode: failureCode, Actor: actor, RecordedAt: recordedAt}
	if allowed {
		event.ReviewProvenance = &provenance
	}
	return event, nil
}

func sameAuthorization(a, b ReviewDecisionProvenance) bool {
	a.DecisionID, b.DecisionID = "", ""
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}
