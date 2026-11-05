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
	ensureCalls  int
	inspectCalls int
	appCalls     int
	exists       bool
	ensureErr    error
	inspectErr   error
	spec         incus.OCIApplicationSpec
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

func TestDeploymentWorkerExecutesOnlyAuthorizedIncusStorageStep(t *testing.T) {
	a, tenantID, _, deployment, job, provider, calls := setupDeploymentStorageExecution(t, true)
	result, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]any)
	step := out["runtime_step"].(map[string]any)
	if out["outcome"] != "runtime_unsupported" || out["provider_mutated"] != true ||
		step["step_state"] != "succeeded" || provider.ensureCalls != 1 || provider.inspectCalls != 1 || *calls != 1 {
		t.Fatalf("worker result=%v ensure=%d inspect=%d decisions=%d", result, provider.ensureCalls, provider.inspectCalls, *calls)
	}
	wantName, _ := incus.OCIApplicationName(deployment.ID)
	if provider.spec.Name != wantName || provider.spec.DeploymentID != deployment.ID || provider.spec.Generation != 1 ||
		provider.spec.StoragePool != "tenant-data" || provider.spec.StorageSizeGiB != 20 || provider.spec.Network != "private-apps" {
		t.Fatalf("provider spec was not built from the reviewed generation: %+v", provider.spec)
	}
	var state string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state FROM shippin_deployment_execution_steps
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=1`, tenantID, deployment.ID).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("step outcome=%s err=%v", state, err)
	}
	var dispatchEvents int
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM shippin_deployment_execution_step_events
		WHERE tenant_id=$1 AND event_type='dispatch_claimed'`, tenantID).Scan(&dispatchEvents); err != nil || dispatchEvents != 1 {
		t.Fatalf("dispatch events=%d err=%v", dispatchEvents, err)
	}
	// A replay sees the completed intent and cannot dispatch the provider again.
	if _, err := a.runDeploymentReconcileJob(context.Background(), job, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if provider.ensureCalls != 1 || provider.inspectCalls != 1 || *calls != 1 {
		t.Fatalf("completed step replay repeated a side effect: ensure=%d inspect=%d decisions=%d", provider.ensureCalls, provider.inspectCalls, *calls)
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
	if out["provider_mutated"] != false || recovery["step_state"] != "succeeded" ||
		provider.ensureCalls != 1 || provider.inspectCalls != 1 {
		t.Fatalf("ambiguous write was not recovered read-only: result=%v ensure=%d inspect=%d", result, provider.ensureCalls, provider.inspectCalls)
	}
	var state string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT state FROM shippin_deployment_execution_steps
		WHERE tenant_id=(SELECT tenant_id FROM shippin_deployments WHERE id=$1) AND deployment_id=$1 AND generation=1`,
		deployment.ID).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("recovered volume step state=%s err=%v", state, err)
	}
}

func setupDeploymentStorageExecution(t *testing.T, allow bool) (*API, string, string, *deployments.Deployment,
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
	_, planSHA, actionSHA, err := deploymentReviewSnapshot(tenantID, deployment)
	if err != nil {
		t.Fatal(err)
	}
	reviewID := uuid.NewString()
	ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
	ticketSHA := strings.Repeat("c", 64)
	if _, _, err := a.deploymentStore().ReserveReview(ctx, tenantID, deployment.ID, 1, "runtime-test-operator",
		uuid.NewString(), reviewID, ticketID, deployments.ExecutionPlanVersion, planSHA, actionSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := a.deploymentStore().SetReviewState(ctx, reviewID, "runtime-test-operator", "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	seedProvenance := deployments.ReviewDecisionProvenance{DecisionID: "initial-human-review", TenantID: agentHubTenant,
		MachineID: machineID, TicketID: ticketID, TicketSHA256: ticketSHA, PolicySHA256: strings.Repeat("b", 64),
		PolicyVersion: 1, MatchedRules: []string{"deployment-review"}, ActionSHA256: actionSHA}
	if _, err := a.deploymentStore().RecordReviewDecision(ctx, tenantID, deployment.ID, 1,
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
		'runtime-test-operator',$4,now()+interval '2 minutes')`, jobID, tenantID, deployment.ID+":1", owner); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(deploymentWakeup{DeploymentID: deployment.ID, Generation: 1})
	job := &jobs.Job{ID: jobID, TenantID: tenantID, Kind: jobs.KindDeploymentReconcile,
		TargetType: "deployment_generation", TargetID: deployment.ID + ":1", CreatedBy: "runtime-test-operator",
		LeaseOwner: owner, Payload: payload}
	provider := &deploymentExecutionProviderFake{exists: true}
	a.hvServers = &memHV{items: []servers.Endpoint{{ID: systemID, Kind: "incus"}}}
	a.openHV = func(servers.Endpoint) (hv.Driver, error) { return deploymentStorageInventoryDriver{}, nil }
	a.hvVisible = func(context.Context, string, string) (servers.Endpoint, error) {
		return servers.Endpoint{ID: systemID, Kind: "incus"}, nil
	}
	a.openIncusOCIExecutor = func(servers.Endpoint) (incusOCIExecutorProvider, error) { return provider, nil }
	return a, tenantID, systemID, deployment, job, provider, &decisions
}
