package deployments

import (
	"context"
	"encoding/json"
	"errors"

	"shippin.cloud/console/internal/catalog"
)

// ExecutionRecoverySnapshot contains only the immutable generation data needed
// to reconcile one ambiguous provider step. Secret references stay references;
// secret material is never resolved by this read path.
type ExecutionRecoverySnapshot struct {
	Deployment        *Deployment
	Definition        DefinitionSnapshot
	Plan              ExecutionPlan
	PlanSHA256        string
	Step              ExecutionStepRecord
	EnvironmentInputs []catalog.RuntimeEnvironmentInput
}

// ExecutionSnapshotWithLease reloads only the current immutable generation
// under a live deployment job lease. It never resolves secrets or performs
// provider I/O, and its plan digest must match the approved generation.
func (s *Store) ExecutionSnapshotWithLease(ctx context.Context, tenantID, deploymentID string, generation int64,
	jobID, leaseOwner, planSHA256 string) (ExecutionRecoverySnapshot, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(jobID) || generation < 1 ||
		!validExecutionLeaseOwner(leaseOwner) || !validSHA256Value(planSHA256) {
		return ExecutionRecoverySnapshot{}, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := checkExecutionLease(ctx, tx, tenantID, deploymentID, generation, jobID, leaseOwner); err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	deployment, err := scanDeployment(tx.QueryRow(ctx, deploymentGenerationSelect+
		` WHERE d.tenant_id=$1 AND d.id=$2 AND g.generation=$3 FOR UPDATE OF d`, tenantID, deploymentID, generation))
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	plan, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	digest, err := plan.Fingerprint()
	if err != nil || digest != planSHA256 {
		return ExecutionRecoverySnapshot{}, ErrReviewConflict
	}
	var reviewed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shippin_deployment_reviews
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3 AND state='pending_review'
		AND execution_plan_version=$4 AND execution_plan_sha256=$5)`,
		tenantID, deploymentID, generation, plan.Version, digest).Scan(&reviewed); err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	if !reviewed {
		return ExecutionRecoverySnapshot{}, ErrReviewConflict
	}
	snapshot, environmentInputs, err := executionSnapshotDefinition(deployment, plan, digest)
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	snapshot.EnvironmentInputs = environmentInputs
	return snapshot, nil
}

// UnresolvedExecutionStepsWithLease returns outstanding intents in approved
// plan order. It performs no provider or secret-manager I/O.
func (s *Store) UnresolvedExecutionStepsWithLease(ctx context.Context, tenantID, deploymentID string, generation int64,
	jobID, leaseOwner string) (ExecutionPlan, string, []ExecutionStepRecord, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(jobID) || generation < 1 || !validExecutionLeaseOwner(leaseOwner) {
		return ExecutionPlan{}, "", nil, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ExecutionPlan{}, "", nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := checkExecutionLease(ctx, tx, tenantID, deploymentID, generation, jobID, leaseOwner); err != nil {
		return ExecutionPlan{}, "", nil, err
	}
	deployment, err := scanDeployment(tx.QueryRow(ctx, deploymentGenerationSelect+
		` WHERE d.tenant_id=$1 AND d.id=$2 AND g.generation=$3 FOR UPDATE OF d`, tenantID, deploymentID, generation))
	if err != nil {
		return ExecutionPlan{}, "", nil, err
	}
	plan, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		return ExecutionPlan{}, "", nil, err
	}
	digest, err := plan.Fingerprint()
	if err != nil {
		return ExecutionPlan{}, "", nil, err
	}
	rows, err := tx.Query(ctx, `SELECT tenant_id::text,deployment_id::text,generation,plan_version,plan_sha256,
		step_id,operation,idempotency_key,state,attempt,coalesce(provider_operation_id,''),coalesce(failure_code,''),
		intent_at,coalesce(completed_at,'epoch'::timestamptz)
		FROM shippin_deployment_execution_steps WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3
		AND plan_sha256=$4 AND state='intent' ORDER BY intent_at,idempotency_key FOR UPDATE`,
		tenantID, deploymentID, generation, digest)
	if err != nil {
		return ExecutionPlan{}, "", nil, err
	}
	byKey := make(map[string]ExecutionStepRecord)
	for rows.Next() {
		record, scanErr := scanExecutionStep(rows)
		if scanErr != nil {
			rows.Close()
			return ExecutionPlan{}, "", nil, scanErr
		}
		byKey[record.IdempotencyKey] = record
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ExecutionPlan{}, "", nil, err
	}
	rows.Close()
	unresolved := make([]ExecutionStepRecord, 0, len(byKey))
	for _, step := range plan.Steps {
		if record, ok := byKey[step.IdempotencyKey]; ok {
			if record.StepID != step.ID || record.Operation != step.Operation {
				return ExecutionPlan{}, "", nil, ErrExecutionStepConflict
			}
			unresolved = append(unresolved, record)
		}
	}
	if len(unresolved) != len(byKey) {
		return ExecutionPlan{}, "", nil, ErrExecutionStepConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionPlan{}, "", nil, err
	}
	return plan, digest, unresolved, nil
}

// ExecutionRecoverySnapshotWithLease reloads a claimed intent from its pinned
// generation. It performs no provider or secret-manager I/O and refuses stale
// generations, plans, claims, or leases.
func (s *Store) ExecutionRecoverySnapshotWithLease(ctx context.Context, tenantID, deploymentID string, generation int64,
	jobID, leaseOwner, planSHA256, idempotencyKey string) (ExecutionRecoverySnapshot, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(jobID) || generation < 1 ||
		!validExecutionLeaseOwner(leaseOwner) || !validSHA256Value(planSHA256) || !validSHA256Value(idempotencyKey) {
		return ExecutionRecoverySnapshot{}, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := checkExecutionLease(ctx, tx, tenantID, deploymentID, generation, jobID, leaseOwner); err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	deployment, err := scanDeployment(tx.QueryRow(ctx, deploymentGenerationSelect+
		` WHERE d.tenant_id=$1 AND d.id=$2 AND g.generation=$3 FOR UPDATE OF d`, tenantID, deploymentID, generation))
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	plan, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	digest, err := plan.Fingerprint()
	if err != nil || digest != planSHA256 {
		return ExecutionRecoverySnapshot{}, ErrReviewConflict
	}
	var step ExecutionStep
	found := false
	for _, candidate := range plan.Steps {
		if candidate.IdempotencyKey == idempotencyKey {
			step, found = candidate, true
			break
		}
	}
	if !found {
		return ExecutionRecoverySnapshot{}, ErrExecutionStepConflict
	}
	record, err := scanExecutionStep(tx.QueryRow(ctx, `SELECT tenant_id::text,deployment_id::text,generation,plan_version,
		plan_sha256,step_id,operation,idempotency_key,state,attempt,coalesce(provider_operation_id,''),
		coalesce(failure_code,''),intent_at,coalesce(completed_at,'epoch'::timestamptz)
		FROM shippin_deployment_execution_steps WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3
		AND idempotency_key=$4 AND plan_sha256=$5 AND state='intent'
		AND recovery_job_id=$6 AND recovery_lease_owner=$7 FOR UPDATE`,
		tenantID, deploymentID, generation, idempotencyKey, digest, jobID, leaseOwner))
	if errors.Is(err, ErrNotFound) {
		return ExecutionRecoverySnapshot{}, ErrExecutionStepConflict
	}
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	if record.StepID != step.ID || record.Operation != step.Operation {
		return ExecutionRecoverySnapshot{}, ErrExecutionStepConflict
	}
	snapshot, environmentInputs, err := executionSnapshotDefinition(deployment, plan, digest)
	if err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	snapshot.Step = record
	snapshot.EnvironmentInputs = environmentInputs
	return snapshot, nil
}

func executionSnapshotDefinition(deployment *Deployment, plan ExecutionPlan, digest string) (ExecutionRecoverySnapshot,
	[]catalog.RuntimeEnvironmentInput, error) {
	var definition DefinitionSnapshot
	if deployment == nil || len(deployment.definitionSnapshot) == 0 ||
		json.Unmarshal(deployment.definitionSnapshot, &definition) != nil || definition.Validate(deployment.Config) != nil {
		return ExecutionRecoverySnapshot{}, nil, ErrDefinitionUnavailable
	}
	environmentInputs := []catalog.RuntimeEnvironmentInput{}
	if definition.Runtime.Version == 3 {
		var err error
		environmentInputs, err = catalog.ExtractRuntimeEnvironmentInputs(definition.Runtime, definition.ConfigSchema, deployment.Config)
		if err != nil {
			return ExecutionRecoverySnapshot{}, nil, ErrDefinitionUnavailable
		}
	}
	return ExecutionRecoverySnapshot{Deployment: deployment, Definition: definition, Plan: plan, PlanSHA256: digest},
		environmentInputs, nil
}
