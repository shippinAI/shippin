package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/servers"
)

type deploymentRecoveryProviderFake struct {
	exists bool
	calls  int
	spec   incus.OCIApplicationSpec
}

func TestDeploymentRecoverySkipsUnsupportedProviderBeforePlanLookup(t *testing.T) {
	api := &API{}
	result, handled, err := api.recoverDeploymentIntent(context.Background(), nil,
		&deployments.Deployment{TargetProvider: deployments.ProviderProxmox})
	if err != nil || handled || result != nil {
		t.Fatalf("Proxmox recovery result=%v handled=%t err=%v", result, handled, err)
	}
}

func (f *deploymentRecoveryProviderFake) InspectOCIApplicationVolume(_ context.Context, spec incus.OCIApplicationSpec) (bool, error) {
	f.calls++
	f.spec = spec
	return f.exists, nil
}

func TestDeploymentOutboxRelayAndWorkerUseGenerationFencedJobs(t *testing.T) {
	a, tenantID, targetID := deploymentRouteAPI(t)
	a.jobs = jobs.NewStore(a.db.Pool)
	a.cfg.Jobs.MaxAttempts = 3
	ctx := context.Background()
	item, _, err := a.deploymentStore().Create(ctx, tenantID, "outbox-test", deployments.CreateInput{
		ServiceID: "postgres", TargetSystemID: targetID, DesiredState: deployments.DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"password":"vaultix://production/services/postgres/password"}`),
		TargetConfig: json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`),
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

func TestDeploymentWorkerRecoversOnlyExactVolumeObservation(t *testing.T) {
	for _, exists := range []bool{true, false} {
		t.Run(map[bool]string{true: "owned_volume", false: "absent_volume"}[exists], func(t *testing.T) {
			a, tenantID, targetID := deploymentRouteAPI(t)
			ctx := context.Background()
			deployment, _, err := a.deploymentStore().Create(ctx, tenantID, "recovery-test", deployments.CreateInput{
				ServiceID: "postgres", TargetSystemID: targetID, DesiredState: deployments.DesiredPresent,
				IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"password":"vaultix://production/services/postgres/password"}`),
				TargetConfig: json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := deployments.BuildExecutionPlan(tenantID, deployment)
			if err != nil || len(plan.Steps) == 0 || plan.Steps[0].Operation != "ensure_persistent_volume" {
				t.Fatalf("execution plan=%+v err=%v", plan, err)
			}
			planDigest, err := plan.Fingerprint()
			if err != nil {
				t.Fatal(err)
			}
			originalJobID, originalOwner := uuid.NewString(), "recovery-original-worker"
			if _, err := a.db.Pool.Exec(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,status,attempts,max_attempts,
				created_by,lease_owner,lease_until) VALUES($1,$2,'deployment.reconcile','deployment_generation',$3,'running',1,3,'test',$4,now()+interval '1 minute')`,
				originalJobID, tenantID, deployment.ID+":1", originalOwner); err != nil {
				t.Fatal(err)
			}
			reviewID := uuid.NewString()
			ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
			if _, _, err := a.deploymentStore().ReserveReview(ctx, tenantID, deployment.ID, 1, "recovery-test", uuid.NewString(), reviewID,
				ticketID, plan.Version, planDigest, strings.Repeat("a", 64)); err != nil {
				t.Fatal(err)
			}
			if _, err := a.deploymentStore().SetReviewState(ctx, reviewID, "recovery-test", "pending_review", strings.Repeat("c", 64), ""); err != nil {
				t.Fatal(err)
			}
			provenance := deployments.ReviewDecisionProvenance{DecisionID: "recovery-decision", TenantID: "agenthub-tenant",
				MachineID: "machine-1", TicketID: ticketID, TicketSHA256: strings.Repeat("c", 64),
				PolicySHA256: strings.Repeat("b", 64), PolicyVersion: 1, MatchedRules: []string{"deployment-review"},
				ActionSHA256: strings.Repeat("a", 64)}
			if _, err := a.deploymentStore().RecordReviewDecision(ctx, tenantID, deployment.ID, 1, "recovery-test", uuid.NewString(), true, provenance, ""); err != nil {
				t.Fatal(err)
			}
			if _, started, err := a.deploymentStore().BeginExecutionStepWithLease(ctx, tenantID, deployment.ID, 1,
				originalJobID, originalOwner, planDigest, plan.Steps[0]); err != nil || !started {
				t.Fatalf("approved intent start=%t err=%v", started, err)
			}
			if _, err := a.db.Pool.Exec(ctx, `UPDATE jobs SET status='failed',lease_owner=NULL,lease_until=NULL WHERE id=$1`, originalJobID); err != nil {
				t.Fatal(err)
			}
			recoveryJobID, recoveryOwner := uuid.NewString(), "recovery-current-worker"
			if _, err := a.db.Pool.Exec(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,status,attempts,max_attempts,
				created_by,lease_owner,lease_until) VALUES($1,$2,'deployment.reconcile','deployment_generation',$3,'running',1,3,'test',$4,now()+interval '1 minute')`,
				recoveryJobID, tenantID, deployment.ID+":1", recoveryOwner); err != nil {
				t.Fatal(err)
			}
			provider := &deploymentRecoveryProviderFake{exists: exists}
			a.hvServers = &memHV{items: []servers.Endpoint{{ID: targetID, Kind: "incus"}}}
			a.openHV = func(servers.Endpoint) (hv.Driver, error) { return deploymentStorageInventoryDriver{}, nil }
			a.hvVisible = func(context.Context, string, string) (servers.Endpoint, error) {
				return servers.Endpoint{ID: targetID, Kind: "incus"}, nil
			}
			a.openIncusOCIRecovery = func(servers.Endpoint) (incusOCIRecoveryProvider, error) { return provider, nil }
			payload, _ := json.Marshal(deploymentWakeup{DeploymentID: deployment.ID, Generation: 1})
			job := &jobs.Job{ID: recoveryJobID, TenantID: tenantID, Kind: jobs.KindDeploymentReconcile,
				TargetType: "deployment_generation", TargetID: deployment.ID + ":1", CreatedBy: "test",
				LeaseOwner: recoveryOwner, Payload: payload}
			result, err := a.runDeploymentReconcileJob(ctx, job, func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			out := result.(map[string]any)
			recovery := out["recovery"].(map[string]any)
			wantState := "succeeded"
			if !exists {
				wantState = "unknown"
			}
			if out["outcome"] != "runtime_unsupported" || out["provider_mutated"] != false || recovery["step_state"] != wantState || provider.calls != 1 {
				t.Fatalf("worker result=%v provider calls=%d", result, provider.calls)
			}
			wantName, _ := incus.OCIApplicationName(deployment.ID)
			if provider.spec.Name != wantName || provider.spec.StoragePool != "tenant-data" || provider.spec.Network != "private-apps" ||
				provider.spec.Generation != 1 || provider.spec.StorageSizeGiB != 20 {
				t.Fatalf("recovery spec=%+v", provider.spec)
			}
			var state, failureCode string
			if err := a.db.Pool.QueryRow(ctx, `SELECT state,coalesce(failure_code,'') FROM shippin_deployment_execution_steps WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, plan.Steps[0].IdempotencyKey).Scan(&state, &failureCode); err != nil {
				t.Fatal(err)
			}
			if state != wantState || exists && failureCode != "" || !exists && failureCode != "provider_state_absent" {
				t.Fatalf("recovered step state=%s failure=%s", state, failureCode)
			}
		})
	}
}
