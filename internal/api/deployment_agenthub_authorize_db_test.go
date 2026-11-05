package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/servers"
)

func TestAuthorizeDeploymentReviewRecordsAllowAndNoMatchDenial(t *testing.T) {
	a, tenantID, systemID := deploymentRouteAPI(t)
	ctx := context.Background()
	a.cfg.SessionKey = strings.Repeat("ab", 32)
	const agentHubTenant, machineID, serviceToken = "agenthub-tenant", "machine-1", "test-service-token"
	if _, err := a.serverStore().SetIdentity(ctx, servers.SystemIdentity{
		ServerID: systemID, Label: "Primary target", AgentHubTenantID: agentHubTenant, AgentHubMachineID: machineID,
	}, "route-test-user"); err != nil {
		t.Fatal(err)
	}
	matched := []string{"review-provider-actions"}
	const policySHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const ticketSHA = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	decisionCalls := 0
	deployment, _, err := a.deploymentStore().Create(ctx, tenantID, "route-test-user", deployments.CreateInput{
		ServiceID: "postgres", TargetSystemID: systemID, DesiredState: deployments.DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"password":"vaultix://production/services/postgres/password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	action := deploymentReviewAction{
		Operation: "deployment.present", TenantID: tenantID, DeploymentID: deployment.ID,
		Generation: deployment.Generation, DesiredState: string(deployment.DesiredState), TargetSystemID: systemID,
		TargetProvider: string(deployment.TargetProvider), ServiceID: deployment.ServiceID,
		DefinitionSHA: deployment.DefinitionSHA256, Source: deployment.Source, Config: deployment.Config,
	}
	actionJSON, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	actionHash := sha256.Sum256(actionJSON)
	actionSHA := hex.EncodeToString(actionHash[:])
	reviewID := uuid.NewString()
	ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
	if _, _, err := a.deploymentStore().ReserveReview(ctx, tenantID, deployment.ID, deployment.Generation,
		"route-test-user", uuid.NewString(), reviewID, ticketID, actionSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := a.deploymentStore().SetReviewState(ctx, reviewID, "route-test-user", "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+serviceToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/policy/doc":
			_, _ = w.Write([]byte(`{"tenant_id":"agenthub-tenant","version":3,"sha256":"` + policySHA + `","doc":{"rules":[{"id":"review-provider-actions","enabled":true,"effect":"require","require":[{"check":"approval"},{"check":"review","min":2,"role":"reviewer"}]}]}}`))
		case "/api/v1/change-tickets/" + ticketID:
			command := deploymentReviewCommand(deployment.ID, deployment.Generation, actionSHA, deployment.DesiredState)
			body, _ := json.Marshal(map[string]any{"ticket": map[string]any{
				"id": ticketID, "tenant_id": agentHubTenant, "machine": machineID, "command": command,
				"fields": map[string]string{"shippin_action_sha256": actionSHA}, "sha256": ticketSHA, "status": "passed", "version": 3,
			}})
			_, _ = w.Write(body)
		case "/api/v1/policy/decide":
			decisionCalls++
			var in struct {
				Tool    string `json:"tool"`
				Command string `json:"command"`
				Machine string `json:"machine"`
				Ticket  string `json:"ticket_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode policy request: %v", err)
			}
			if in.Tool != "shippin:deployment.apply" || in.Machine != machineID || in.Ticket != ticketID || in.Command != deploymentReviewCommand(deployment.ID, deployment.Generation, actionSHA, deployment.DesiredState) {
				t.Errorf("authorization request was not bound to exact deployment: %+v", in)
			}
			body, _ := json.Marshal(map[string]any{
				"decision_id": "decision-123", "tenant_id": agentHubTenant, "policy_sha256": policySHA, "allow": true,
				"decision": map[string]any{"allow": true, "policy_version": 3, "matched": matched},
			})
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
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
	if err := a.db.SaveAgentHubConnection(ctx, server.URL, ciphertext, false, "route-test-user"); err != nil {
		t.Fatal(err)
	}
	request := func(key string) *httptest.ResponseRecorder {
		req := deploymentRequest(http.MethodPost, tenantID, deployment.ID,
			`{"generation":1}`, key, "")
		req.SetPathValue("deploymentId", deployment.ID)
		rec := httptest.NewRecorder()
		a.authorizeDeploymentReview(rec, req)
		return rec
	}
	managerResolver := a.resolveCaller
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return &caller{id: &auth.Identity{Sub: "route-test-member"}, tenants: []tenantView{{Tenant: db.Tenant{ID: tenantID}, Roles: []string{"org_member"}}}}, true
	}
	forbidden := request(uuid.NewString())
	if forbidden.Code != http.StatusForbidden || decisionCalls != 0 {
		t.Fatalf("tenant member authorization check: status=%d decision calls=%d body=%s", forbidden.Code, decisionCalls, forbidden.Body)
	}
	a.resolveCaller = managerResolver
	wrongGeneration := deploymentRequest(http.MethodPost, tenantID, deployment.ID, `{"generation":2}`, uuid.NewString(), "")
	wrongGeneration.SetPathValue("deploymentId", deployment.ID)
	wrongGenerationRecorder := httptest.NewRecorder()
	a.authorizeDeploymentReview(wrongGenerationRecorder, wrongGeneration)
	if wrongGenerationRecorder.Code != http.StatusPreconditionFailed || decisionCalls != 0 {
		t.Fatalf("stale generation must be rejected before AgentHub: status=%d calls=%d", wrongGenerationRecorder.Code, decisionCalls)
	}
	allowed := request(uuid.NewString())
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), `"event_type":"review_allowed"`) ||
		!strings.Contains(allowed.Body.String(), `"policy_version":3`) || strings.Contains(allowed.Body.String(), serviceToken) || decisionCalls != 1 {
		t.Fatalf("authorization allow response: %d %s", allowed.Code, allowed.Body)
	}
	matched = []string{}
	denied := request(uuid.NewString())
	if denied.Code != http.StatusConflict || !strings.Contains(denied.Body.String(), `"event_type":"review_denied"`) {
		t.Fatalf("no-match authorization response: %d %s", denied.Code, denied.Body)
	}
	events, err := a.deploymentStore().Events(ctx, tenantID, deployment.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	var allowedEvent, deniedEvent bool
	for _, event := range events {
		allowedEvent = allowedEvent || event.Type == deployments.EventReviewAllowed && event.ReviewProvenance != nil && event.ReviewProvenance.ActionSHA256 == actionSHA
		deniedEvent = deniedEvent || event.Type == deployments.EventReviewDenied && event.FailureCode == "agenthub_review_denied"
	}
	if !allowedEvent || !deniedEvent {
		t.Fatalf("authorization decisions missing from durable event history: %+v", events)
	}
	current, err := a.deploymentStore().Get(ctx, tenantID, deployment.ID)
	if err != nil || current.ObservedState != deployments.ObservedUnknown {
		t.Fatalf("authorization check must not mutate provider-observed state: current=%+v err=%v", current, err)
	}
}
