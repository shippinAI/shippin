package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/jobs"
)

func TestDeploymentOutboxRelayAndWorkerUseGenerationFencedJobs(t *testing.T) {
	a, tenantID, targetID := deploymentRouteAPI(t)
	a.jobs = jobs.NewStore(a.db.Pool)
	a.cfg.Jobs.MaxAttempts = 3
	ctx := context.Background()
	item, _, err := a.deploymentStore().Create(ctx, tenantID, "outbox-test", deployments.CreateInput{
		ServiceID: "postgres", TargetSystemID: targetID, DesiredState: deployments.DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"password":"vaultix://production/services/postgres/password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Pool.Exec(ctx, `CREATE FUNCTION shippin_test_reject_outbox_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'simulated relay crash before receipt'; END; $$;
		CREATE TRIGGER shippin_test_reject_outbox_receipt BEFORE UPDATE ON shippin_deployment_outbox
		FOR EACH ROW EXECUTE FUNCTION shippin_test_reject_outbox_receipt()`); err != nil {
		t.Fatal(err)
	}
	a.relayDeploymentOutbox(ctx)
	firstTarget := item.ID + ":1"
	first, err := a.jobs.Live(ctx, jobs.KindDeploymentReconcile, "deployment_generation", firstTarget)
	if err != nil {
		t.Fatalf("first generation not relayed: %v", err)
	}
	if pending, err := a.deploymentStore().PendingOutbox(ctx, 10); err != nil || len(pending) != 1 {
		t.Fatalf("failed publication receipt must leave the wake-up pending: %+v err=%v", pending, err)
	}
	// Simulate recovery after the enqueue committed but receipt publication did not.
	if _, err := a.db.Pool.Exec(ctx, `DROP TRIGGER shippin_test_reject_outbox_receipt ON shippin_deployment_outbox;
		DROP FUNCTION shippin_test_reject_outbox_receipt()`); err != nil {
		t.Fatal(err)
	}
	a.relayDeploymentOutbox(ctx)
	firstReplay, err := a.jobs.Live(ctx, jobs.KindDeploymentReconcile, "deployment_generation", firstTarget)
	if err != nil || firstReplay.ID != first.ID {
		t.Fatalf("relay recovery must reuse the live idempotent job: first=%+v replay=%+v err=%v", first, firstReplay, err)
	}
	if pending, err := a.deploymentStore().PendingOutbox(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("recovered receipt still pending: %+v err=%v", pending, err)
	}
	updated, _, err := a.deploymentStore().Update(ctx, tenantID, item.ID, "outbox-test", 1, deployments.UpdateInput{
		DesiredState: deployments.DesiredPresent, IdempotencyKey: uuid.NewString(),
		Config: json.RawMessage(`{"password":"vaultix://production/services/postgres/password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	a.relayDeploymentOutbox(ctx)
	second, err := a.jobs.Live(ctx, jobs.KindDeploymentReconcile, "deployment_generation", item.ID+":2")
	if err != nil {
		t.Fatalf("new generation must receive a distinct job: %v", err)
	}
	second.LeaseOwner = "deployment-outbox-test-worker"
	if _, err := a.db.Pool.Exec(ctx, `UPDATE jobs SET status='running',lease_owner=$2,lease_until=now()-interval '1 second' WHERE id=$1`,
		second.ID, second.LeaseOwner); err != nil {
		t.Fatal(err)
	}
	var steps []string
	result, err := a.runDeploymentReconcileJob(ctx, first, func(step string) { steps = append(steps, step) })
	if err != nil || result.(map[string]any)["outcome"] != "superseded" {
		t.Fatalf("stale job should coalesce without mutation: result=%v err=%v", result, err)
	}
	result, err = a.runDeploymentReconcileJob(ctx, second, func(step string) { steps = append(steps, step) })
	if !errors.Is(err, deployments.ErrLostJobLease) {
		t.Fatalf("expired worker lease must not persist deployment failure: result=%v err=%v", result, err)
	}
	current, err := a.deploymentStore().Get(ctx, tenantID, item.ID)
	if err != nil || current.ObservedState != deployments.ObservedPending {
		t.Fatalf("expired worker changed observed state: deployment=%+v err=%v", current, err)
	}
	if _, err := a.db.Pool.Exec(ctx, `UPDATE jobs SET lease_owner='replacement-worker',lease_until=now()+interval '1 minute' WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.runDeploymentReconcileJob(ctx, second, func(step string) { steps = append(steps, step) }); !errors.Is(err, deployments.ErrLostJobLease) {
		t.Fatalf("replaced worker lease must not persist deployment failure: err=%v", err)
	}
	if _, err := a.db.Pool.Exec(ctx, `UPDATE jobs SET lease_owner=$2,lease_until=now()+interval '1 minute' WHERE id=$1`, second.ID, second.LeaseOwner); err != nil {
		t.Fatal(err)
	}
	result, err = a.runDeploymentReconcileJob(ctx, second, func(step string) { steps = append(steps, step) })
	if err != nil || result.(map[string]any)["outcome"] != "runtime_unsupported" || result.(map[string]any)["provider_mutated"] != false {
		t.Fatalf("current unsupported runtime outcome: result=%v err=%v", result, err)
	}
	current, err = a.deploymentStore().Get(ctx, tenantID, item.ID)
	if err != nil || current.Generation != updated.Generation || current.ObservedState != deployments.ObservedFailed {
		t.Fatalf("worker must persist explicit runtime limitation: deployment=%+v err=%v", current, err)
	}
	events, err := a.deploymentStore().Events(ctx, tenantID, item.ID, 20)
	if err != nil || len(events) != 3 || events[2].FailureCode != "runtime_unsupported" || events[2].Generation != 2 {
		t.Fatalf("worker event history: events=%+v err=%v", events, err)
	}
}
