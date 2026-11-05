package deployments

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrReviewConflict = errors.New("deployment review conflicts with the current generation")

type ReviewRequest struct {
	ID                   string    `json:"id"`
	DeploymentID         string    `json:"deployment_id"`
	TenantID             string    `json:"tenant_id"`
	Generation           int64     `json:"generation"`
	ExecutionPlanVersion int       `json:"execution_plan_version,omitempty"`
	ExecutionPlanSHA256  string    `json:"execution_plan_sha256,omitempty"`
	ActionSHA256         string    `json:"action_sha256"`
	AgentHubTicket       string    `json:"agenthub_ticket_id"`
	State                string    `json:"state"`
	TicketSHA256         string    `json:"ticket_sha256,omitempty"`
	FailureCode          string    `json:"failure_code,omitempty"`
	CreatedBy            string    `json:"created_by"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// ReserveReview creates one stable Shippin-to-AgentHub correlation for an
// immutable deployment generation. It never calls AgentHub or a provider.
func (s *Store) ReserveReview(ctx context.Context, tenantID, deploymentID string, generation int64,
	actor, idempotencyKey, reviewID, ticketID string, executionPlanVersion int, executionPlanSHA256, actionSHA256 string) (ReviewRequest, bool, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(idempotencyKey) || !validUUID(reviewID) ||
		generation < 1 || !validAgentHubTicketID(ticketID) || executionPlanVersion != ExecutionPlanVersion || !validSHA256Value(executionPlanSHA256) ||
		!validSHA256Value(actionSHA256) || !validReviewActor(actor) {
		return ReviewRequest{}, false, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReviewRequest{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenantID+":"+deploymentID+":"+strconv.FormatInt(generation, 10)); err != nil {
		return ReviewRequest{}, false, err
	}
	var current int64
	if err := tx.QueryRow(ctx, `SELECT desired_generation FROM shippin_deployments WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, deploymentID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReviewRequest{}, false, ErrNotFound
		}
		return ReviewRequest{}, false, err
	}
	if current != generation {
		return ReviewRequest{}, false, ErrGenerationConflict
	}
	prior, err := scanReview(tx.QueryRow(ctx, reviewSelect+` WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3 FOR UPDATE`, tenantID, deploymentID, generation))
	if err == nil {
		if prior.ExecutionPlanVersion != executionPlanVersion || prior.ExecutionPlanSHA256 != executionPlanSHA256 || prior.ActionSHA256 != actionSHA256 {
			return ReviewRequest{}, false, ErrReviewConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return ReviewRequest{}, false, err
		}
		return prior, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ReviewRequest{}, false, err
	}
	var conflicting string
	err = tx.QueryRow(ctx, `SELECT id::text FROM shippin_deployment_reviews WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(&conflicting)
	if err == nil {
		return ReviewRequest{}, false, ErrIdempotencyConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReviewRequest{}, false, err
	}
	var review ReviewRequest
	err = tx.QueryRow(ctx, `INSERT INTO shippin_deployment_reviews
		(id,tenant_id,deployment_id,generation,idempotency_key,execution_plan_version,execution_plan_sha256,action_sha256,agenthub_ticket_id,state,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'preparing',$10) RETURNING `+reviewColumns,
		reviewID, tenantID, deploymentID, generation, idempotencyKey, executionPlanVersion, executionPlanSHA256, actionSHA256, ticketID, actor).Scan(
		&review.ID, &review.TenantID, &review.DeploymentID, &review.Generation, &review.ExecutionPlanVersion, &review.ExecutionPlanSHA256, &review.ActionSHA256, &review.AgentHubTicket,
		&review.State, &review.TicketSHA256, &review.FailureCode, &review.CreatedBy, &review.CreatedAt, &review.UpdatedAt)
	if err != nil {
		return ReviewRequest{}, false, err
	}
	if err := appendReviewEvent(ctx, tx, review.ID, review.State, "", "", actor); err != nil {
		return ReviewRequest{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewRequest{}, false, err
	}
	return review, true, nil
}

const reviewColumns = `id::text,tenant_id::text,deployment_id::text,generation,coalesce(execution_plan_version,0),coalesce(execution_plan_sha256,''),action_sha256,agenthub_ticket_id,state,
  coalesce(ticket_sha256,''),coalesce(failure_code,''),created_by,created_at,updated_at`
const reviewSelect = `SELECT ` + reviewColumns + ` FROM shippin_deployment_reviews`

func scanReview(row pgx.Row) (ReviewRequest, error) {
	var item ReviewRequest
	err := row.Scan(&item.ID, &item.TenantID, &item.DeploymentID, &item.Generation, &item.ExecutionPlanVersion, &item.ExecutionPlanSHA256,
		&item.ActionSHA256, &item.AgentHubTicket,
		&item.State, &item.TicketSHA256, &item.FailureCode, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewRequest{}, ErrNotFound
	}
	return item, err
}

func (s *Store) GetReview(ctx context.Context, tenantID, deploymentID string, generation int64) (ReviewRequest, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || generation < 1 {
		return ReviewRequest{}, ErrNotFound
	}
	return scanReview(s.pool.QueryRow(ctx, reviewSelect+` WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3`, tenantID, deploymentID, generation))
}

// SetReviewState persists only a redacted workflow state and AgentHub ticket
// digest. Human reviews and their content remain exclusively in AgentHub.
func (s *Store) SetReviewState(ctx context.Context, reviewID, actor, state, ticketSHA256, failureCode string) (ReviewRequest, error) {
	if !validUUID(reviewID) || !validReviewActor(actor) || len(ticketSHA256) != 0 && !validSHA256Value(ticketSHA256) ||
		!validReviewState(state) || !validFailureCode(failureCode) {
		return ReviewRequest{}, ErrInvalidRequest
	}
	if state == "pending_review" && (ticketSHA256 == "" || failureCode != "") || state == "failed" && failureCode == "" || state == "preparing" && failureCode != "" {
		return ReviewRequest{}, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReviewRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var review ReviewRequest
	if _, err := tx.Exec(ctx, `UPDATE shippin_deployment_reviews SET state=$2,ticket_sha256=nullif($3,''),failure_code=nullif($4,''),updated_at=now() WHERE id=$1`, reviewID, state, ticketSHA256, failureCode); err != nil {
		return ReviewRequest{}, err
	}
	review, err = scanReview(tx.QueryRow(ctx, reviewSelect+` WHERE id=$1`, reviewID))
	if err != nil {
		return ReviewRequest{}, err
	}
	if err := appendReviewEvent(ctx, tx, reviewID, state, ticketSHA256, failureCode, actor); err != nil {
		return ReviewRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewRequest{}, err
	}
	return review, nil
}

func appendReviewEvent(ctx context.Context, tx pgx.Tx, reviewID, state, ticketSHA256, failureCode, actor string) error {
	_, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_review_events(id,review_id,state,ticket_sha256,failure_code,actor)
		VALUES($1,$2,$3,nullif($4,''),nullif($5,''),$6)`, uuid.NewString(), reviewID, state, ticketSHA256, failureCode, actor)
	return err
}

func validReviewState(state string) bool {
	return state == "preparing" || state == "pending_review" || state == "failed"
}
func validReviewActor(actor string) bool {
	return len(actor) > 0 && len(actor) <= 255 && strings.TrimSpace(actor) == actor && !strings.ContainsAny(actor, "\r\n\x00")
}
func validFailureCode(value string) bool {
	if value == "" {
		return true
	}
	return len(value) <= 80 && failureCodePattern.MatchString(value)
}
func validAgentHubTicketID(value string) bool {
	if len(value) < 4 || len(value) > 64 || !strings.HasPrefix(value, "shp-") {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
func validSHA256Value(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
