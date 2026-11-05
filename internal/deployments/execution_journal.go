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

var ErrExecutionStepConflict = errors.New("deployment execution step conflicts with its recorded intent")

// ExecutionStepRecord is the bounded Shippin journal projection for one
// approved plan step. It intentionally contains no request configuration or
// provider response body.
type ExecutionStepRecord struct {
	TenantID            string    `json:"tenant_id"`
	DeploymentID        string    `json:"deployment_id"`
	Generation          int64     `json:"generation"`
	PlanVersion         int       `json:"plan_version"`
	PlanSHA256          string    `json:"plan_sha256"`
	StepID              string    `json:"step_id"`
	Operation           string    `json:"operation"`
	IdempotencyKey      string    `json:"idempotency_key"`
	State               string    `json:"state"`
	Attempt             int       `json:"attempt"`
	ProviderOperationID string    `json:"provider_operation_id,omitempty"`
	FailureCode         string    `json:"failure_code,omitempty"`
	IntentAt            time.Time `json:"intent_at"`
	CompletedAt         time.Time `json:"completed_at,omitempty"`
}

// BeginExecutionStepWithLease durably records an approved step before a caller
// may issue its provider effect. Replays return the existing record with
// started=false; an unfinished intent is never permission to dispatch again.
func (s *Store) BeginExecutionStepWithLease(ctx context.Context, tenantID, deploymentID string, generation int64,
	jobID, leaseOwner, planSHA256 string, step ExecutionStep) (ExecutionStepRecord, bool, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(jobID) || generation < 1 ||
		!validExecutionLeaseOwner(leaseOwner) || !validSHA256Value(planSHA256) ||
		step.ID == "" || step.Operation == "" || !validSHA256Value(step.IdempotencyKey) {
		return ExecutionStepRecord{}, false, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ExecutionStepRecord{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := checkExecutionLease(ctx, tx, tenantID, deploymentID, generation, jobID, leaseOwner); err != nil {
		return ExecutionStepRecord{}, false, err
	}
	deployment, err := scanDeployment(tx.QueryRow(ctx, deploymentGenerationSelect+
		` WHERE d.tenant_id=$1 AND d.id=$2 AND g.generation=$3 FOR UPDATE OF d`, tenantID, deploymentID, generation))
	if err != nil {
		return ExecutionStepRecord{}, false, err
	}
	plan, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		return ExecutionStepRecord{}, false, err
	}
	digest, err := plan.Fingerprint()
	if err != nil || digest != planSHA256 {
		return ExecutionStepRecord{}, false, ErrReviewConflict
	}
	if !planContainsStep(plan, step) {
		return ExecutionStepRecord{}, false, ErrInvalidRequest
	}
	var approved bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM shippin_deployment_reviews r
		JOIN shippin_deployment_events e ON e.tenant_id=r.tenant_id AND e.deployment_id=r.deployment_id
			AND e.generation=r.generation AND e.event_type='review_allowed'
		WHERE r.tenant_id=$1 AND r.deployment_id=$2 AND r.generation=$3
			AND r.execution_plan_version=$4 AND r.execution_plan_sha256=$5
			AND e.review_provenance->>'action_sha256'=r.action_sha256
		)`, tenantID, deploymentID, generation, plan.Version, digest).Scan(&approved)
	if err != nil {
		return ExecutionStepRecord{}, false, err
	}
	if !approved {
		return ExecutionStepRecord{}, false, ErrReviewConflict
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+step.IdempotencyKey); err != nil {
		return ExecutionStepRecord{}, false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_execution_steps
		(tenant_id,deployment_id,generation,plan_version,plan_sha256,step_id,operation,idempotency_key,state,intent_job_id,intent_lease_owner)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'intent',$9,$10) ON CONFLICT DO NOTHING`,
		tenantID, deploymentID, generation, plan.Version, digest, step.ID, step.Operation, step.IdempotencyKey, jobID, leaseOwner)
	if err != nil {
		return ExecutionStepRecord{}, false, err
	}
	started := tag.RowsAffected() == 1
	if started {
		if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_execution_step_events
			(id,tenant_id,idempotency_key,event_type,actor) VALUES($1,$2,$3,'intent',$4)`, uuid.NewString(), tenantID, step.IdempotencyKey, leaseOwner); err != nil {
			return ExecutionStepRecord{}, false, err
		}
	}
	record, err := scanExecutionStep(tx.QueryRow(ctx, `SELECT tenant_id::text,deployment_id::text,generation,plan_version,
		plan_sha256,step_id,operation,idempotency_key,state,attempt,coalesce(provider_operation_id,''),
		coalesce(failure_code,''),intent_at,coalesce(completed_at,'epoch'::timestamptz)
		FROM shippin_deployment_execution_steps WHERE tenant_id=$1 AND
			(idempotency_key=$2 OR (deployment_id=$3 AND generation=$4 AND step_id=$5)) FOR UPDATE`,
		tenantID, step.IdempotencyKey, deploymentID, generation, step.ID))
	if err != nil {
		return ExecutionStepRecord{}, false, err
	}
	if record.DeploymentID != deploymentID || record.Generation != generation || record.PlanSHA256 != digest ||
		record.StepID != step.ID || record.Operation != step.Operation || record.IdempotencyKey != step.IdempotencyKey {
		return ExecutionStepRecord{}, false, ErrExecutionStepConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionStepRecord{}, false, err
	}
	return record, started, nil
}

// RecordExecutionStepOutcomeWithLease closes a previously persisted intent.
// A worker that lost its lease cannot report a provider result; the next worker
// must first reconcile the ambiguous intent by reading provider state.
func (s *Store) RecordExecutionStepOutcomeWithLease(ctx context.Context, tenantID, deploymentID string, generation int64,
	jobID, leaseOwner, planSHA256, idempotencyKey, state, providerOperationID, failureCode string) (ExecutionStepRecord, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(jobID) || generation < 1 ||
		!validExecutionLeaseOwner(leaseOwner) || !validSHA256Value(planSHA256) ||
		!validSHA256Value(idempotencyKey) || (state != "succeeded" && state != "failed" && state != "unknown") ||
		providerOperationID != "" && !providerOperationIDPattern.MatchString(providerOperationID) || strings.Contains(providerOperationID, "://") ||
		!validFailureCode(failureCode) || state == "failed" && failureCode == "" || state == "succeeded" && failureCode != "" {
		return ExecutionStepRecord{}, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ExecutionStepRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := checkExecutionLease(ctx, tx, tenantID, deploymentID, generation, jobID, leaseOwner); err != nil {
		return ExecutionStepRecord{}, err
	}
	var planVersion int
	err = tx.QueryRow(ctx, `SELECT plan_version FROM shippin_deployment_execution_steps
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3 AND idempotency_key=$4
		AND plan_sha256=$5 AND intent_job_id=$6 AND intent_lease_owner=$7 AND state IN ('intent','succeeded','failed','unknown') FOR UPDATE`,
		tenantID, deploymentID, generation, idempotencyKey, planSHA256, jobID, leaseOwner).Scan(&planVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExecutionStepRecord{}, ErrExecutionStepConflict
	}
	if err != nil {
		return ExecutionStepRecord{}, err
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT state FROM shippin_deployment_execution_steps WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(&existing)
	if err != nil {
		return ExecutionStepRecord{}, err
	}
	if existing == "intent" {
		_, err = tx.Exec(ctx, `UPDATE shippin_deployment_execution_steps SET state=$3,
			provider_operation_id=NULLIF($4,''),failure_code=NULLIF($5,''),completed_at=now()
			WHERE tenant_id=$1 AND idempotency_key=$2 AND state='intent'`, tenantID, idempotencyKey, state, providerOperationID, failureCode)
		if err != nil {
			return ExecutionStepRecord{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_execution_step_events
			(id,tenant_id,idempotency_key,event_type,actor,provider_operation_id,failure_code)
			VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''))`, uuid.NewString(), tenantID, idempotencyKey, state, leaseOwner, providerOperationID, failureCode); err != nil {
			return ExecutionStepRecord{}, err
		}
	} else if existing != state {
		return ExecutionStepRecord{}, ErrExecutionStepConflict
	}
	record, err := scanExecutionStep(tx.QueryRow(ctx, `SELECT tenant_id::text,deployment_id::text,generation,plan_version,
		plan_sha256,step_id,operation,idempotency_key,state,attempt,coalesce(provider_operation_id,''),
		coalesce(failure_code,''),intent_at,coalesce(completed_at,'epoch'::timestamptz)
		FROM shippin_deployment_execution_steps WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey))
	if err != nil {
		return ExecutionStepRecord{}, err
	}
	if record.State != state || record.ProviderOperationID != providerOperationID || record.FailureCode != failureCode {
		return ExecutionStepRecord{}, ErrExecutionStepConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionStepRecord{}, err
	}
	if planVersion != ExecutionPlanVersion {
		return ExecutionStepRecord{}, ErrExecutionStepConflict
	}
	return record, nil
}

func scanExecutionStep(row pgx.Row) (ExecutionStepRecord, error) {
	var item ExecutionStepRecord
	var completed time.Time
	err := row.Scan(&item.TenantID, &item.DeploymentID, &item.Generation, &item.PlanVersion, &item.PlanSHA256,
		&item.StepID, &item.Operation, &item.IdempotencyKey, &item.State, &item.Attempt, &item.ProviderOperationID,
		&item.FailureCode, &item.IntentAt, &completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExecutionStepRecord{}, ErrNotFound
	}
	if completed.Unix() != 0 {
		item.CompletedAt = completed
	}
	return item, err
}

func checkExecutionLease(ctx context.Context, tx pgx.Tx, tenantID, deploymentID string, generation int64,
	jobID, leaseOwner string) error {
	target := deploymentID + ":" + strconv.FormatInt(generation, 10)
	var owned string
	err := tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE id=$1 AND tenant_id=$2
		AND kind='deployment.reconcile' AND target_type='deployment_generation' AND target_id=$3
		AND status='running' AND lease_owner=$4 AND lease_until>now() FOR UPDATE`, jobID, tenantID, target, leaseOwner).Scan(&owned)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLostJobLease
	}
	if err != nil {
		return err
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT desired_generation FROM shippin_deployments WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, deploymentID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != generation {
		return ErrGenerationConflict
	}
	return nil
}

func planContainsStep(plan ExecutionPlan, step ExecutionStep) bool {
	for _, candidate := range plan.Steps {
		if candidate == step {
			return true
		}
	}
	return false
}

func validExecutionLeaseOwner(owner string) bool {
	return len(owner) > 0 && len(owner) <= 255 && strings.TrimSpace(owner) == owner && !strings.ContainsAny(owner, "\r\n\x00")
}
