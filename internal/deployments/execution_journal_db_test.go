package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestExecutionStepIntentIsApprovedLeaseFencedAndNeverRedispatched(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `UPDATE catalog_services SET runtime_contract=$1::jsonb WHERE id='postgres'`,
		`{"version":3,"kind":"incus_oci_container","providers":["incus"],"resources":{"cpu_cores":2,"memory_mib":2048},"storage":{"size_gib":20,"mount_path":"/var/lib/postgresql/data"},"network":{"exposure":"private","ports":[{"name":"postgres","protocol":"tcp","container_port":5432}]},"health":{"kind":"tcp","port":5432,"interval_seconds":30,"timeout_seconds":5,"failure_threshold":3},"environment":[{"name":"POSTGRES_PASSWORD","config_path":"/password"}]}`); err != nil {
		t.Fatal(err)
	}
	deployment, _, err := store.Create(ctx, tenantID, "operator@example.test", CreateInput{
		ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`),
		TargetConfig: json.RawMessage(`{"storage":{"pool":"tenant-pool"},"network":{"name":"tenant-net"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil || plan.UnsupportedReason != "executor_not_installed" || len(plan.Steps) == 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	planSHA, err := plan.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps[0]
	jobID, owner := uuid.NewString(), "worker-a"
	if _, err := store.pool.Exec(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,status,attempts,max_attempts,
		created_by,lease_owner,lease_until) VALUES($1,$2,'deployment.reconcile','deployment_generation',$3,'running',1,3,'test',$4,now()+interval '1 minute')`,
		jobID, tenantID, deployment.ID+":1", owner); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.BeginExecutionStepWithLease(ctx, tenantID, deployment.ID, 1, jobID, owner, planSHA, step); !errors.Is(err, ErrReviewConflict) {
		t.Fatalf("unapproved intent accepted: %v", err)
	}
	reviewID := uuid.NewString()
	ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
	actionSHA := strings.Repeat("a", 64)
	if _, _, err := store.ReserveReview(ctx, tenantID, deployment.ID, 1, "operator@example.test", uuid.NewString(), reviewID, ticketID,
		plan.Version, planSHA, actionSHA); err != nil {
		t.Fatal(err)
	}
	ticketSHA := strings.Repeat("c", 64)
	if _, err := store.SetReviewState(ctx, reviewID, "operator@example.test", "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	provenance := ReviewDecisionProvenance{DecisionID: "decision-step", TenantID: "agenthub-tenant", MachineID: "machine-1", TicketID: ticketID,
		TicketSHA256: ticketSHA, PolicySHA256: strings.Repeat("b", 64), PolicyVersion: 1, MatchedRules: []string{"deployment-review"}, ActionSHA256: actionSHA}
	if _, err := store.RecordReviewDecision(ctx, tenantID, deployment.ID, 1, "operator@example.test", uuid.NewString(), true, provenance, ""); err != nil {
		t.Fatal(err)
	}

	intent, started, err := store.BeginExecutionStepWithLease(ctx, tenantID, deployment.ID, 1, jobID, owner, planSHA, step)
	if err != nil || !started || intent.State != "intent" || intent.Attempt != 1 {
		t.Fatalf("intent=%+v started=%t err=%v", intent, started, err)
	}
	unresolvedPlan, unresolvedDigest, unresolved, err := store.UnresolvedExecutionStepsWithLease(ctx, tenantID,
		deployment.ID, 1, jobID, owner)
	if err != nil || unresolvedDigest != planSHA || unresolvedPlan.Version != plan.Version || len(unresolved) != 1 || unresolved[0].IdempotencyKey != step.IdempotencyKey {
		t.Fatalf("unresolved steps plan=%+v digest=%s steps=%+v err=%v", unresolvedPlan, unresolvedDigest, unresolved, err)
	}
	replayed, started, err := store.BeginExecutionStepWithLease(ctx, tenantID, deployment.ID, 1, jobID, owner, planSHA, step)
	if err != nil || started || replayed.IdempotencyKey != intent.IdempotencyKey {
		t.Fatalf("replay=%+v started=%t err=%v", replayed, started, err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE jobs SET status='failed',lease_owner=NULL,lease_until=NULL WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	recoveryJobID, recoveryOwner := uuid.NewString(), "worker-recovery"
	if _, err := store.pool.Exec(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,status,attempts,max_attempts,
		created_by,lease_owner,lease_until) VALUES($1,$2,'deployment.reconcile','deployment_generation',$3,'running',1,3,'test',$4,now()+interval '1 minute')`,
		recoveryJobID, tenantID, deployment.ID+":1", recoveryOwner); err != nil {
		t.Fatal(err)
	}
	claimed, didClaim, err := store.ClaimExecutionStepRecoveryWithLease(ctx, tenantID, deployment.ID, 1,
		recoveryJobID, recoveryOwner, planSHA, step)
	if err != nil || !didClaim || claimed.State != "intent" {
		t.Fatalf("recovery claim=%+v didClaim=%t err=%v", claimed, didClaim, err)
	}
	claimed, didClaim, err = store.ClaimExecutionStepRecoveryWithLease(ctx, tenantID, deployment.ID, 1,
		recoveryJobID, recoveryOwner, planSHA, step)
	if err != nil || didClaim || claimed.State != "intent" {
		t.Fatalf("recovery claim replay=%+v didClaim=%t err=%v", claimed, didClaim, err)
	}
	reclaimedOwner := "worker-recovery-reclaimed"
	if _, err := store.pool.Exec(ctx, `UPDATE jobs SET lease_owner=$2,lease_until=now()+interval '1 minute' WHERE id=$1`, recoveryJobID, reclaimedOwner); err != nil {
		t.Fatal(err)
	}
	claimed, didClaim, err = store.ClaimExecutionStepRecoveryWithLease(ctx, tenantID, deployment.ID, 1,
		recoveryJobID, reclaimedOwner, planSHA, step)
	if err != nil || !didClaim || claimed.State != "intent" {
		t.Fatalf("reclaimed recovery claim=%+v didClaim=%t err=%v", claimed, didClaim, err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE jobs SET status='failed',lease_owner=NULL,lease_until=NULL WHERE id=$1`, recoveryJobID); err != nil {
		t.Fatal(err)
	}
	finalJobID, finalOwner := uuid.NewString(), "worker-recovery-final"
	if _, err := store.pool.Exec(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,status,attempts,max_attempts,
		created_by,lease_owner,lease_until) VALUES($1,$2,'deployment.reconcile','deployment_generation',$3,'running',1,3,'test',$4,now()+interval '1 minute')`,
		finalJobID, tenantID, deployment.ID+":1", finalOwner); err != nil {
		t.Fatal(err)
	}
	claimed, didClaim, err = store.ClaimExecutionStepRecoveryWithLease(ctx, tenantID, deployment.ID, 1,
		finalJobID, finalOwner, planSHA, step)
	if err != nil || !didClaim || claimed.State != "intent" {
		t.Fatalf("new-job recovery takeover=%+v didClaim=%t err=%v", claimed, didClaim, err)
	}
	recoverySnapshot, err := store.ExecutionRecoverySnapshotWithLease(ctx, tenantID, deployment.ID, 1,
		finalJobID, finalOwner, planSHA, step.IdempotencyKey)
	if err != nil || recoverySnapshot.Step.State != "intent" || recoverySnapshot.Definition.Runtime.Version != 3 ||
		recoverySnapshot.PlanSHA256 != planSHA || len(recoverySnapshot.EnvironmentInputs) != 1 ||
		recoverySnapshot.EnvironmentInputs[0].VaultixReference != "vaultix://production/services/postgres/password" ||
		recoverySnapshot.EnvironmentInputs[0].Value != nil {
		t.Fatalf("runtime recovery snapshot=%+v err=%v", recoverySnapshot, err)
	}
	if _, err := store.ExecutionRecoverySnapshotWithLease(ctx, tenantID, deployment.ID, 1,
		finalJobID, finalOwner, strings.Repeat("f", 64), step.IdempotencyKey); !errors.Is(err, ErrReviewConflict) {
		t.Fatalf("stale plan loaded recovery snapshot: %v", err)
	}
	if _, err := store.ExecutionRecoverySnapshotWithLease(ctx, tenantID, deployment.ID, 1,
		recoveryJobID, reclaimedOwner, planSHA, step.IdempotencyKey); !errors.Is(err, ErrLostJobLease) {
		t.Fatalf("stale recovery lease loaded snapshot: %v", err)
	}
	if _, err := store.RecordExecutionStepOutcomeWithLease(ctx, tenantID, deployment.ID, 1, recoveryJobID, recoveryOwner, planSHA, step.IdempotencyKey, "succeeded", "provider-op-1", ""); !errors.Is(err, ErrLostJobLease) {
		t.Fatalf("replaced recovery lease wrote a result: %v", err)
	}
	if _, err := store.RecordExecutionStepOutcomeWithLease(ctx, tenantID, deployment.ID, 1, recoveryJobID, reclaimedOwner, planSHA, step.IdempotencyKey, "succeeded", "provider-op-1", ""); !errors.Is(err, ErrLostJobLease) {
		t.Fatalf("finished recovery job wrote a result: %v", err)
	}
	if _, err := store.RecordExecutionStepOutcomeWithLease(ctx, tenantID, deployment.ID, 1, jobID, owner, planSHA, step.IdempotencyKey, "succeeded", "provider-op-1", ""); !errors.Is(err, ErrLostJobLease) {
		t.Fatalf("replaced lease wrote a result: %v", err)
	}
	completed, err := store.RecordExecutionStepOutcomeWithLease(ctx, tenantID, deployment.ID, 1, finalJobID, finalOwner, planSHA, step.IdempotencyKey, "succeeded", "provider-op-1", "")
	if err != nil || completed.State != "succeeded" || completed.ProviderOperationID != "provider-op-1" {
		t.Fatalf("outcome=%+v err=%v", completed, err)
	}
	replayed, started, err = store.BeginExecutionStepWithLease(ctx, tenantID, deployment.ID, 1, finalJobID, finalOwner, planSHA, step)
	if err != nil || started || replayed.State != "succeeded" {
		t.Fatalf("completed step replay=%+v started=%t err=%v", replayed, started, err)
	}
	var eventCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM shippin_deployment_execution_step_events WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, step.IdempotencyKey).Scan(&eventCount); err != nil || eventCount != 5 {
		t.Fatalf("expected intent, recovery claims and outcome events, count=%d err=%v", eventCount, err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE shippin_deployment_execution_step_events SET actor='changed' WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, step.IdempotencyKey); err == nil {
		t.Fatal("execution step history was mutable")
	}
}
