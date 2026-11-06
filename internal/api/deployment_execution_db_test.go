package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/servers"
)

type deploymentExecutionProviderFake struct {
	ensureCalls   int
	inspectCalls  int
	appCalls      int
	createCalls   int
	runCalls      int
	stopCalls     int
	deleteCalls   int
	exists        bool
	deleteErr     error
	hasSnapshots  bool
	powerExists   bool
	powerStatus   string
	usePowerState bool
	ensureErr     error
	inspectErr    error
	createErr     error
	createStatus  string
	runStatus     string
	spec          incus.OCIApplicationSpec
}

func (f *deploymentExecutionProviderFake) EnsureOCIApplicationVolume(_ context.Context, spec incus.OCIApplicationSpec) error {
	f.ensureCalls++
	f.spec = spec
	return f.ensureErr
}

func (f *deploymentExecutionProviderFake) InspectOCIApplicationVolume(_ context.Context, spec incus.OCIApplicationSpec) (bool, error) {
	f.inspectCalls++
	f.spec = spec
	return f.exists, f.inspectErr
}

func (f *deploymentExecutionProviderFake) InspectOCIApplication(_ context.Context, spec incus.OCIApplicationSpec) (hv.Guest, bool, error) {
	f.appCalls++
	f.spec = spec
	return hv.Guest{ID: spec.Name, Status: "Stopped"}, false, nil
}

func (f *deploymentExecutionProviderFake) InspectOCIApplicationPowerState(_ context.Context, spec incus.OCIApplicationSpec) (hv.Guest, bool, error) {
	f.appCalls++
	f.spec = spec
	if f.usePowerState {
		return hv.Guest{ID: spec.Name, Status: f.powerStatus}, f.powerExists, nil
	}
	return hv.Guest{ID: spec.Name, Status: "Running"}, false, nil
}

func (f *deploymentExecutionProviderFake) InspectOCIApplicationSnapshots(_ context.Context, spec incus.OCIApplicationSpec) (bool, error) {
	f.spec = spec
	return f.hasSnapshots, nil
}

func (f *deploymentExecutionProviderFake) StopOCIApplication(_ context.Context, spec incus.OCIApplicationSpec) error {
	f.stopCalls++
	f.spec = spec
	f.powerStatus = "Stopped"
	return nil
}

func (f *deploymentExecutionProviderFake) DeleteOCIApplicationStopped(_ context.Context, spec incus.OCIApplicationSpec) error {
	f.deleteCalls++
	f.spec = spec
	f.powerExists = false
	return f.deleteErr
}

func (f *deploymentExecutionProviderFake) CreateOCIApplicationStopped(_ context.Context, spec incus.OCIApplicationSpec) (hv.Guest, error) {
	f.createCalls++
	f.spec = spec
	if f.createErr != nil {
		return hv.Guest{}, f.createErr
	}
	status := f.createStatus
	if status == "" {
		status = "Stopped"
	}
	return hv.Guest{ID: spec.Name, Status: status}, nil
}

func (f *deploymentExecutionProviderFake) EnsureOCIApplicationRunning(_ context.Context, spec incus.OCIApplicationSpec) (hv.Guest, error) {
	f.runCalls++
	f.spec = spec
	status := f.runStatus
	if status == "" {
		status = "Running"
	}
	return hv.Guest{ID: spec.Name, Status: status}, nil
}

func TestDeploymentWorkerExecutesAuthorizedIncusStorageAndStoppedWorkloadSteps(t *testing.T) {
	a, tenantID, _, deployment, job, provider, calls := setupDeploymentStorageExecution(t, true)
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["outcome"] != "runtime_unsupported" || out["provider_mutated"] != true ||
		step["step_state"] != "unknown" || step["step_id"] != "health" || step["health_status"] != "unknown" ||
		step["application_probe_verified"] != false || step["pending"] != true || provider.ensureCalls != 1 ||
		provider.inspectCalls != 1 || provider.createCalls != 1 || provider.runCalls != 1 || *calls != 3 {
		t.Fatalf("worker result=%v ensure=%d inspect=%d create=%d run=%d decisions=%d", result, provider.ensureCalls, provider.inspectCalls, provider.createCalls, provider.runCalls, *calls)
	}
	wantName, _ := incus.OCIApplicationName(deployment.ID)
	if provider.spec.Name != wantName || provider.spec.DeploymentID != deployment.ID || provider.spec.Generation != 1 ||
		provider.spec.StoragePool != "tenant-data" || provider.spec.StorageSizeGiB != 20 || provider.spec.Network != "private-apps" {
		t.Fatalf("provider spec was not built from the reviewed generation: %+v", provider.spec)
	}
	var state string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state FROM shippin_deployment_execution_steps
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=1 AND step_id='health'`, tenantID, deployment.ID).Scan(&state); err != nil || state != "intent" {
		t.Fatalf("step outcome=%s err=%v", state, err)
	}
	var dispatchEvents int
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM shippin_deployment_execution_step_events
		WHERE tenant_id=$1 AND event_type='dispatch_claimed'`, tenantID).Scan(&dispatchEvents); err != nil || dispatchEvents != 3 {
		t.Fatalf("dispatch events=%d err=%v", dispatchEvents, err)
	}
	var completedSteps int
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM shippin_deployment_execution_steps
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=1 AND state='succeeded'`, tenantID, deployment.ID).Scan(&completedSteps); err != nil || completedSteps != 3 {
		t.Fatalf("completed steps=%d err=%v", completedSteps, err)
	}
	var observed string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT observed_state FROM shippin_deployments
		WHERE tenant_id=$1 AND id=$2`, tenantID, deployment.ID).Scan(&observed); err != nil || observed != string(deployments.ObservedFailed) {
		t.Fatalf("deployment without an application probe must stay unsupported: observed state=%q err=%v", observed, err)
	}
	var healthEvents int
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM shippin_deployment_events
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=1 AND event_type='health_observed' AND health_status='passing'`, tenantID, deployment.ID).Scan(&healthEvents); err != nil || healthEvents != 0 {
		t.Fatalf("unprobed deployment recorded %d passing health events, err=%v", healthEvents, err)
	}
	// A replay sees the completed intent and cannot dispatch the provider again.
	if _, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if provider.ensureCalls != 1 || provider.inspectCalls != 1 || provider.createCalls != 1 || provider.runCalls != 1 || *calls != 3 {
		t.Fatalf("completed step replay repeated a side effect: ensure=%d inspect=%d create=%d run=%d decisions=%d", provider.ensureCalls, provider.inspectCalls, provider.createCalls, provider.runCalls, *calls)
	}
}

func TestDeploymentWorkerAgentHubDenialDoesNotWriteIncus(t *testing.T) {
	a, _, _, deployment, job, provider, calls := setupDeploymentStorageExecution(t, false)
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["provider_mutated"] != false || step["provider_dispatched"] != false || step["step_state"] != "failed" ||
		provider.ensureCalls != 0 || provider.inspectCalls != 0 || *calls != 1 {
		t.Fatalf("denied worker result=%v ensure=%d inspect=%d decisions=%d", result, provider.ensureCalls, provider.inspectCalls, *calls)
	}
	var state, failure string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state,coalesce(failure_code,'') FROM shippin_deployment_execution_steps
		WHERE deployment_id=$1 AND generation=1`, deployment.ID).Scan(&state, &failure); err != nil || state != "failed" || failure != "agenthub_review_denied" {
		t.Fatalf("denied step state=%s failure=%s err=%v", state, failure, err)
	}
}

func TestDeploymentWorkerExecutesReviewedIncusRemovalAndPreservesVolume(t *testing.T) {
	a, tenantID, _, deployment, job, provider, decisions := setupDeploymentRemovalExecution(t, true)
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["outcome"] != "absent" || out["provider_mutated"] != true || step["step_id"] != "readback" ||
		step["step_state"] != "succeeded" || provider.stopCalls != 1 || provider.deleteCalls != 1 ||
		provider.powerExists || !provider.exists || *decisions != 2 {
		t.Fatalf("removal result=%v stop=%d delete=%d appExists=%v volumeExists=%v decisions=%d",
			result, provider.stopCalls, provider.deleteCalls, provider.powerExists, provider.exists, *decisions)
	}
	var observed string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT observed_state FROM shippin_deployments WHERE tenant_id=$1 AND id=$2`,
		tenantID, deployment.ID).Scan(&observed); err != nil || observed != string(deployments.ObservedAbsent) {
		t.Fatalf("deployment observed state=%q err=%v", observed, err)
	}
	var completed, dispatched int
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM shippin_deployment_execution_steps
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3 AND state='succeeded'`, tenantID, deployment.ID, deployment.Generation).Scan(&completed); err != nil || completed != 3 {
		t.Fatalf("completed removal steps=%d err=%v", completed, err)
	}
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM shippin_deployment_execution_step_events e
		JOIN shippin_deployment_execution_steps s USING(tenant_id,idempotency_key)
		WHERE s.tenant_id=$1 AND s.deployment_id=$2 AND s.generation=$3 AND e.event_type='dispatch_claimed'`,
		tenantID, deployment.ID, deployment.Generation).Scan(&dispatched); err != nil || dispatched != 2 {
		t.Fatalf("provider mutation dispatch claims=%d err=%v", dispatched, err)
	}
	if _, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if provider.stopCalls != 1 || provider.deleteCalls != 1 || *decisions != 2 {
		t.Fatalf("completed removal replay repeated provider action or review: stop=%d delete=%d decisions=%d",
			provider.stopCalls, provider.deleteCalls, *decisions)
	}
}

func TestDeploymentWorkerAgentHubDenialDoesNotStopOrDeleteIncus(t *testing.T) {
	a, _, _, deployment, job, provider, decisions := setupDeploymentRemovalExecution(t, false)
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["provider_mutated"] != false || step["step_id"] != "power" || step["step_state"] != "failed" ||
		step["failure_code"] != "agenthub_review_denied" || provider.stopCalls != 0 || provider.deleteCalls != 0 ||
		!provider.powerExists || *decisions != 1 {
		t.Fatalf("denied removal result=%v stop=%d delete=%d exists=%v decisions=%d",
			result, provider.stopCalls, provider.deleteCalls, provider.powerExists, *decisions)
	}
	var state, failure string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state,coalesce(failure_code,'') FROM shippin_deployment_execution_steps
		WHERE tenant_id=(SELECT tenant_id FROM shippin_deployments WHERE id=$1) AND deployment_id=$1 AND generation=$2 AND step_id='power'`,
		deployment.ID, deployment.Generation).Scan(&state, &failure); err != nil || state != "failed" || failure != "agenthub_review_denied" {
		t.Fatalf("denied removal step state=%s failure=%s err=%v", state, failure, err)
	}
}

func TestDeploymentWorkerRefusesIncusRemovalWithSnapshotsBeforeDeleteDispatch(t *testing.T) {
	a, tenantID, _, deployment, job, provider, decisions := setupDeploymentRemovalExecution(t, true)
	provider.hasSnapshots = true
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["outcome"] != "runtime_unsupported" || out["provider_mutated"] != true ||
		step["step_id"] != "workload" || step["step_state"] != "failed" ||
		step["failure_code"] != "snapshots_present" || provider.stopCalls != 1 || provider.deleteCalls != 0 || *decisions != 2 {
		t.Fatalf("snapshot refusal result=%v stop=%d delete=%d decisions=%d", result, provider.stopCalls, provider.deleteCalls, *decisions)
	}
	var deleteDispatches int
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM shippin_deployment_execution_step_events e
		JOIN shippin_deployment_execution_steps s USING(tenant_id,idempotency_key)
		WHERE s.tenant_id=$1 AND s.deployment_id=$2 AND s.generation=$3 AND s.step_id='workload' AND e.event_type='dispatch_claimed'`,
		tenantID, deployment.ID, deployment.Generation).Scan(&deleteDispatches); err != nil || deleteDispatches != 0 {
		t.Fatalf("snapshot-blocked delete dispatches=%d err=%v", deleteDispatches, err)
	}
}

func TestDeploymentWorkerRecoversAmbiguousIncusRemovalWithoutRetry(t *testing.T) {
	a, _, _, deployment, job, provider, decisions := setupDeploymentRemovalExecution(t, true)
	a.openIncusOCIRecovery = func(servers.Endpoint) (incusOCIRecoveryProvider, error) { return provider, nil }
	provider.deleteErr = errors.New("simulated timeout after delete")
	if _, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {}); err == nil {
		t.Fatal("ambiguous delete should leave its intent unresolved")
	}
	if provider.stopCalls != 1 || provider.deleteCalls != 1 || provider.powerExists {
		t.Fatalf("first removal attempt did not preserve ambiguous delete: stop=%d delete=%d exists=%v", provider.stopCalls, provider.deleteCalls, provider.powerExists)
	}
	provider.deleteErr = nil
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	recovery := out["recovery"].(map[string]any)
	if out["outcome"] != "absent" || recovery["step_id"] != "workload" || recovery["step_state"] != "succeeded" ||
		provider.stopCalls != 1 || provider.deleteCalls != 1 || *decisions != 2 {
		t.Fatalf("ambiguous delete was retried or not recovered: result=%v stop=%d delete=%d decisions=%d",
			result, provider.stopCalls, provider.deleteCalls, *decisions)
	}
	var failure string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT coalesce(failure_code,'') FROM shippin_deployment_execution_steps
		WHERE tenant_id=(SELECT tenant_id FROM shippin_deployments WHERE id=$1) AND deployment_id=$1 AND generation=$2 AND step_id='workload'`,
		deployment.ID, deployment.Generation).Scan(&failure); err != nil || failure != "" {
		t.Fatalf("recovered delete failure=%q err=%v", failure, err)
	}
}

func TestDeploymentWorkerRejectsRunningReadbackForStoppedWorkloadStep(t *testing.T) {
	a, _, _, deployment, job, provider, _ := setupDeploymentStorageExecution(t, true)
	provider.createStatus = "Running"
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["outcome"] != "runtime_unsupported" || step["step_id"] != "workload" ||
		step["step_state"] != "unknown" || step["failure_code"] != "provider_state_mismatch" ||
		provider.createCalls != 1 {
		t.Fatalf("unexpected running read-back was accepted: result=%v create calls=%d", result, provider.createCalls)
	}
	var state, failure string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state,coalesce(failure_code,'') FROM shippin_deployment_execution_steps
		WHERE tenant_id=(SELECT tenant_id FROM shippin_deployments WHERE id=$1) AND deployment_id=$1 AND generation=1 AND step_id='workload'`,
		deployment.ID).Scan(&state, &failure); err != nil || state != "unknown" || failure != "provider_state_mismatch" {
		t.Fatalf("workload read-back state=%s failure=%s err=%v", state, failure, err)
	}
}

func TestDeploymentWorkerRejectsNonRunningReadbackForPowerStep(t *testing.T) {
	a, _, _, deployment, job, provider, _ := setupDeploymentStorageExecution(t, true)
	provider.runStatus = "Stopped"
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["outcome"] != "runtime_unsupported" || step["step_id"] != "power" ||
		step["step_state"] != "unknown" || step["failure_code"] != "provider_state_mismatch" || provider.runCalls != 1 {
		t.Fatalf("unexpected power read-back was accepted: result=%v power calls=%d", result, provider.runCalls)
	}
	var state, failure string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state,coalesce(failure_code,'') FROM shippin_deployment_execution_steps
		WHERE tenant_id=(SELECT tenant_id FROM shippin_deployments WHERE id=$1) AND deployment_id=$1 AND generation=1 AND step_id='power'`,
		deployment.ID).Scan(&state, &failure); err != nil || state != "unknown" || failure != "provider_state_mismatch" {
		t.Fatalf("power read-back state=%s failure=%s err=%v", state, failure, err)
	}
}

func TestDeploymentWorkerRecoversAmbiguousIncusVolumeWriteWithoutRetry(t *testing.T) {
	a, _, _, deployment, job, provider, _ := setupDeploymentStorageExecution(t, true)
	provider.exists = true // Model a provider timeout after the create completed.
	provider.ensureErr = errors.New("simulated provider timeout")
	a.openIncusOCIRecovery = func(servers.Endpoint) (incusOCIRecoveryProvider, error) { return provider, nil }
	if _, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {}); err == nil {
		t.Fatal("provider error should leave an ambiguous intent for read-only recovery")
	}
	if provider.ensureCalls != 1 || provider.inspectCalls != 0 {
		t.Fatalf("first attempt did not stop at ambiguous provider error: ensure=%d inspect=%d", provider.ensureCalls, provider.inspectCalls)
	}
	provider.ensureErr = nil
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	recovery := out["recovery"].(map[string]any)
	if out["provider_mutated"] != true || recovery["step_state"] != "succeeded" ||
		provider.ensureCalls != 1 || provider.inspectCalls != 1 || provider.createCalls != 1 {
		t.Fatalf("ambiguous write was not recovered before workload creation: result=%v ensure=%d inspect=%d create=%d", result, provider.ensureCalls, provider.inspectCalls, provider.createCalls)
	}
	var state string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state FROM shippin_deployment_execution_steps
		WHERE tenant_id=(SELECT tenant_id FROM shippin_deployments WHERE id=$1) AND deployment_id=$1 AND generation=1`,
		deployment.ID).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("recovered volume step state=%s err=%v", state, err)
	}
}

func TestDeploymentWorkerRecoversAmbiguousIncusWorkloadCreateWithoutRetry(t *testing.T) {
	a, _, _, deployment, job, provider, _ := setupDeploymentStorageExecution(t, true)
	provider.createErr = errors.New("simulated create timeout")
	a.openIncusOCIRecovery = func(servers.Endpoint) (incusOCIRecoveryProvider, error) { return provider, nil }
	if _, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {}); err == nil {
		t.Fatal("provider create error should leave an ambiguous workload intent")
	}
	if provider.createCalls != 1 || provider.appCalls != 0 {
		t.Fatalf("first attempt did not stop at ambiguous create: creates=%d inspections=%d", provider.createCalls, provider.appCalls)
	}
	provider.createErr = nil
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	recovery := out["recovery"].(map[string]any)
	if out["provider_mutated"] != false || recovery["step_state"] != "unknown" ||
		provider.createCalls != 1 || provider.appCalls != 1 {
		t.Fatalf("ambiguous create retried or was not marked unknown: result=%v creates=%d inspections=%d", result, provider.createCalls, provider.appCalls)
	}
	var state, failure string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state,coalesce(failure_code,'') FROM shippin_deployment_execution_steps
		WHERE tenant_id=(SELECT tenant_id FROM shippin_deployments WHERE id=$1) AND deployment_id=$1 AND generation=1 AND step_id='workload'`,
		deployment.ID).Scan(&state, &failure); err != nil || state != "unknown" || failure != "provider_state_absent" {
		t.Fatalf("ambiguous workload outcome state=%s failure=%s err=%v", state, failure, err)
	}
}

func setupDeploymentStorageExecution(t *testing.T, allow bool) (*API, string, string, *deployments.Deployment,
	*jobs.Job, *deploymentExecutionProviderFake, *int) {
	return setupDeploymentExecution(t, allow, deployments.DesiredPresent)
}

func setupDeploymentRemovalExecution(t *testing.T, allow bool) (*API, string, string, *deployments.Deployment,
	*jobs.Job, *deploymentExecutionProviderFake, *int) {
	return setupDeploymentExecution(t, allow, deployments.DesiredAbsent)
}

func setupDeploymentExecution(t *testing.T, allow bool, desired deployments.DesiredState) (*API, string, string, *deployments.Deployment,
	*jobs.Job, *deploymentExecutionProviderFake, *int) {
	t.Helper()
	a, tenantID, systemID := deploymentRouteAPI(t)
	ctx := context.Background()
	a.cfg.SessionKey = strings.Repeat("ab", 32)
	const agentHubTenant, machineID, serviceToken = "agenthub-tenant", "machine-1", "test-service-token"
	if _, err := a.serverStore().SetIdentity(ctx, servers.SystemIdentity{ServerID: systemID,
		Label: "Execution target", AgentHubTenantID: agentHubTenant, AgentHubMachineID: machineID}, "runtime-test-operator"); err != nil {
		t.Fatal(err)
	}
	deployment, _, err := a.deploymentStore().Create(ctx, tenantID, "runtime-test-operator", deployments.CreateInput{
		ServiceID: "postgres", TargetSystemID: systemID, DesiredState: deployments.DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"password":"vaultix://production/services/postgres/password"}`),
		TargetConfig: json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if desired == deployments.DesiredAbsent {
		deployment, _, err = a.deploymentStore().Update(ctx, tenantID, deployment.ID, "runtime-test-operator", deployment.Generation,
			deployments.UpdateInput{DesiredState: deployments.DesiredAbsent, IdempotencyKey: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, planSHA, actionSHA, err := deploymentReviewSnapshot(tenantID, deployment)
	if err != nil {
		t.Fatal(err)
	}
	reviewID := uuid.NewString()
	ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
	ticketSHA := strings.Repeat("c", 64)
	if _, _, err := a.deploymentStore().ReserveReview(ctx, tenantID, deployment.ID, deployment.Generation, "runtime-test-operator",
		uuid.NewString(), reviewID, ticketID, deployments.ExecutionPlanVersion, planSHA, actionSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := a.deploymentStore().SetReviewState(ctx, reviewID, "runtime-test-operator", "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	seedProvenance := deployments.ReviewDecisionProvenance{DecisionID: "initial-human-review", TenantID: agentHubTenant,
		MachineID: machineID, TicketID: ticketID, TicketSHA256: ticketSHA, PolicySHA256: strings.Repeat("b", 64),
		PolicyVersion: 1, MatchedRules: []string{"deployment-review"}, ActionSHA256: actionSHA}
	if _, err := a.deploymentStore().RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
		"runtime-test-operator", uuid.NewString(), true, seedProvenance, ""); err != nil {
		t.Fatal(err)
	}
	decisions := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+serviceToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/policy/doc":
			_, _ = w.Write([]byte(`{"tenant_id":"agenthub-tenant","version":3,"sha256":"` + strings.Repeat("b", 64) + `","doc":{"rules":[{"id":"deployment-review","enabled":true,"effect":"require","require":[{"check":"approval"},{"check":"review","min":2,"role":"reviewer"}]}]}}`))
		case "/api/v1/change-tickets/" + ticketID:
			command := deploymentReviewCommand(deployment.ID, deployment.Generation, actionSHA, deployment.DesiredState)
			body, _ := json.Marshal(map[string]any{"ticket": map[string]any{"id": ticketID, "tenant_id": agentHubTenant,
				"machine": machineID, "command": command, "fields": map[string]string{
					"shippin_action_sha256": actionSHA, "shippin_execution_plan_sha256": planSHA,
					"shippin_execution_plan_version": strconv.Itoa(deployments.ExecutionPlanVersion)},
				"sha256": ticketSHA, "status": "passed", "version": 3}})
			_, _ = w.Write(body)
		case "/api/v1/policy/decide":
			decisions++
			decision, effect := "true", "true"
			if !allow {
				decision, effect = "false", "false"
			}
			body := `{"decision_id":"worker-decision","tenant_id":"agenthub-tenant","policy_sha256":"` + strings.Repeat("b", 64) +
				`","allow":` + decision + `,"decision":{"allow":` + effect + `,"policy_version":3,"matched":["deployment-review"]}}`
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	priorTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = priorTransport })
	sealer, err := a.sealer()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := sealer.Seal([]byte(serviceToken), agentHubCredentialAAD)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.SaveAgentHubConnection(ctx, server.URL, ciphertext, false, "runtime-test-operator"); err != nil {
		t.Fatal(err)
	}
	owner, jobID := "runtime-test-worker", uuid.NewString()
	if _, err := a.db.Pool.Exec(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,status,attempts,max_attempts,
		created_by,lease_owner,lease_until) VALUES($1,$2,'deployment.reconcile','deployment_generation',$3,'running',1,3,
		'runtime-test-operator',$4,now()+interval '2 minutes')`, jobID, tenantID, deployment.ID+":"+strconv.FormatInt(deployment.Generation, 10), owner); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(deploymentWakeup{DeploymentID: deployment.ID, Generation: deployment.Generation})
	job := &jobs.Job{ID: jobID, TenantID: tenantID, Kind: jobs.KindDeploymentReconcile,
		TargetType: "deployment_generation", TargetID: deployment.ID + ":" + strconv.FormatInt(deployment.Generation, 10), CreatedBy: "runtime-test-operator",
		LeaseOwner: owner, Payload: payload}
	provider := &deploymentExecutionProviderFake{exists: true, powerExists: true, powerStatus: "Running", usePowerState: true}
	a.hvServers = &memHV{items: []servers.Endpoint{{ID: systemID, Kind: "incus"}}}
	a.openHV = func(servers.Endpoint) (hv.Driver, error) { return deploymentStorageInventoryDriver{}, nil }
	a.hvVisible = func(context.Context, string, string) (servers.Endpoint, error) {
		return servers.Endpoint{ID: systemID, Kind: "incus"}, nil
	}
	a.openIncusOCIExecutor = func(servers.Endpoint) (incusOCIExecutorProvider, error) { return provider, nil }
	return a, tenantID, systemID, deployment, job, provider, &decisions
}
