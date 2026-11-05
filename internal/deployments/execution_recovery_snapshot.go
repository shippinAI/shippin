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
	var definition DefinitionSnapshot
	if len(deployment.definitionSnapshot) == 0 || json.Unmarshal(deployment.definitionSnapshot, &definition) != nil || definition.Validate(deployment.Config) != nil {
		return ExecutionRecoverySnapshot{}, ErrDefinitionUnavailable
	}
	environmentInputs := []catalog.RuntimeEnvironmentInput{}
	if definition.Runtime.Version == 3 {
		environmentInputs, err = catalog.ExtractRuntimeEnvironmentInputs(definition.Runtime, definition.ConfigSchema, deployment.Config)
		if err != nil {
			return ExecutionRecoverySnapshot{}, ErrDefinitionUnavailable
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionRecoverySnapshot{}, err
	}
	return ExecutionRecoverySnapshot{Deployment: deployment, Definition: definition, Plan: plan,
		PlanSHA256: digest, Step: record, EnvironmentInputs: environmentInputs}, nil
}
