package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agenthub"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/servers"
)

const (
	powerReviewAgentHubTenant = "power-review-tenant"
	powerReviewMachine        = "power-review-machine"
	powerReviewToken          = "power-review-service-token"
	powerReviewPolicySHA      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	powerReviewTicketSHA      = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

type powerReviewDriver struct{ calls atomic.Int32 }

func (d *powerReviewDriver) Kind() string { return "incus" }
func (*powerReviewDriver) Health(context.Context) (hv.Health, error) {
	return hv.Health{Online: true}, nil
}
func (*powerReviewDriver) Guests(context.Context) ([]hv.Guest, error) {
	return []hv.Guest{{ID: "guest-a", Status: "running"}}, nil
}
func (*powerReviewDriver) Guest(_ context.Context, id string) (hv.Guest, error) {
	return hv.Guest{ID: id, Name: "guest-a", Status: "running"}, nil
}
func (d *powerReviewDriver) Start(_ context.Context, id string) (hv.Guest, error) {
	d.calls.Add(1)
	return hv.Guest{ID: id, Status: "running"}, nil
}
func (d *powerReviewDriver) Stop(_ context.Context, id string) (hv.Guest, error) {
	d.calls.Add(1)
	return hv.Guest{ID: id, Status: "stopped"}, nil
}
func (d *powerReviewDriver) Shutdown(_ context.Context, id string) (hv.Guest, error) {
	d.calls.Add(1)
	return hv.Guest{ID: id, Status: "stopped"}, nil
}
func (d *powerReviewDriver) Reboot(_ context.Context, id string) (hv.Guest, error) {
	d.calls.Add(1)
	return hv.Guest{ID: id, Status: "running"}, nil
}
func (*powerReviewDriver) Snapshot(context.Context, string, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrUnsupported
}
func (*powerReviewDriver) Storage(context.Context) ([]hv.Volume, error)   { return nil, nil }
func (*powerReviewDriver) Networks(context.Context) ([]hv.Network, error) { return nil, nil }

func TestInfraGuestPowerRequiresCurrentAgentHubReviewAndConsumesOnce(t *testing.T) {
	a, _, serverID := deploymentRouteAPI(t)
	ctx := t.Context()
	a.cfg.SessionKey = strings.Repeat("ab", 32)
	if _, err := a.serverStore().SetIdentity(ctx, servers.SystemIdentity{ServerID: serverID, Label: "test target", AgentHubTenantID: powerReviewAgentHubTenant, AgentHubMachineID: powerReviewMachine}, "review-test"); err != nil {
		t.Fatal(err)
	}
	ep := servers.Endpoint{ID: serverID, Kind: "incus", BaseURL: "https://incus.example.test", Node: "default", TokenEnv: "SHIPPIN_INCUS_TOKEN", SealedSecret: []byte("ciphertext")}
	dir := &memHV{items: []servers.Endpoint{ep}}
	a.hvServers = dir
	driver := &powerReviewDriver{}
	a.openHV = func(servers.Endpoint) (hv.Driver, error) { return driver, nil }
	var status atomic.Value
	status.Store("pending_review")
	var createCalls, submitCalls atomic.Int32
	var ticketID, ticketCommand, actionSHA string
	currentTicketSHA := powerReviewTicketSHA
	ticketFields := map[string]string{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		policy := `{"tenant_id":"` + powerReviewAgentHubTenant + `","version":3,"sha256":"` + powerReviewPolicySHA + `","doc":{"rules":[{"id":"guest-power-review","enabled":true,"effect":"require","require":[{"check":"approval"},{"check":"review","min":2,"role":"reviewer"}]}]}}`
		switch {
		case r.URL.Path == "/api/v1/policy/doc":
			_, _ = w.Write([]byte(policy))
		case r.URL.Path == "/api/v1/machines/"+powerReviewMachine:
			_, _ = w.Write([]byte(`{"id":"` + powerReviewMachine + `"}`))
		case r.URL.Path == "/api/v1/policy/decide":
			var in struct {
				Command string `json:"command"`
				DryRun  bool   `json:"dry_run"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode decision: %v", err)
			}
			if in.DryRun {
				_, _ = w.Write([]byte(`{"decision_id":"mapping","tenant_id":"` + powerReviewAgentHubTenant + `","policy_sha256":"` + powerReviewPolicySHA + `","allow":true,"decision":{"allow":true,"policy_version":3,"matched":["guest-power-review"]}}`))
				return
			}
			var cmd struct {
				ActionSHA string `json:"action_sha256"`
			}
			_ = json.Unmarshal([]byte(in.Command), &cmd)
			actionSHA = cmd.ActionSHA
			_, _ = w.Write([]byte(`{"decision_id":"decision-power-1","tenant_id":"` + powerReviewAgentHubTenant + `","policy_sha256":"` + powerReviewPolicySHA + `","allow":true,"decision":{"allow":true,"policy_version":3,"matched":["guest-power-review"]}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/change-tickets":
			var draft agenthub.TicketDraft
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
				t.Errorf("decode ticket: %v", err)
			}
			ticketID, ticketCommand, actionSHA = draft.ID, draft.Command, draft.Fields["shippin_action_sha256"]
			ticketFields = draft.Fields
			createCalls.Add(1)
			writePowerReviewTicket(w, ticketID, powerReviewAgentHubTenant, powerReviewMachine, ticketCommand, actionSHA, status.Load().(string), currentTicketSHA, ticketFields)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/evidence"):
			writePowerReviewTicket(w, ticketID, powerReviewAgentHubTenant, powerReviewMachine, ticketCommand, actionSHA, status.Load().(string), currentTicketSHA, ticketFields)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/submit"):
			submitCalls.Add(1)
			writePowerReviewTicket(w, ticketID, powerReviewAgentHubTenant, powerReviewMachine, ticketCommand, actionSHA, "pending_review", currentTicketSHA, ticketFields)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/change-tickets/"+ticketID:
			writePowerReviewEnvelope(w, ticketID, powerReviewAgentHubTenant, powerReviewMachine, ticketCommand, actionSHA, status.Load().(string), currentTicketSHA, ticketFields)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	prior := http.DefaultTransport
	transport := server.Client().Transport.(*http.Transport).Clone()
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = prior; transport.CloseIdleConnections() })
	sealer, err := a.sealer()
	if err != nil {
		t.Fatal(err)
	}
	secret, err := sealer.Seal([]byte(powerReviewToken), agentHubCredentialAAD)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.SaveAgentHubConnection(ctx, server.URL, secret, false, "review-test"); err != nil {
		t.Fatal(err)
	}
	c := &caller{id: &auth.Identity{Sub: "review-test-user", Name: "Review Operator"}, op: "op_infra", note: "Restart guest after maintenance"}
	prepare := func(key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
		r.Header.Set("Idempotency-Key", key)
		r.SetPathValue("id", serverID)
		r.SetPathValue("guest", "guest-a")
		r.SetPathValue("verb", "reboot")
		w := httptest.NewRecorder()
		a.prepareInfraPowerReview(w, r, c)
		return w
	}
	key := uuid.NewString()
	requested := prepare(key)
	if requested.Code != http.StatusAccepted || createCalls.Load() != 1 || submitCalls.Load() != 1 {
		t.Fatalf("review request %d %s creates=%d submits=%d", requested.Code, requested.Body, createCalls.Load(), submitCalls.Load())
	}
	var prepared struct {
		Review servers.PowerReview `json:"review"`
	}
	if err := json.Unmarshal(requested.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Review.State != "pending_review" || prepared.Review.ActionSHA256 == "" || prepared.Review.TicketSHA256 != powerReviewTicketSHA {
		t.Fatalf("review was not durably prepared: %+v", prepared.Review)
	}
	replayed := prepare(key)
	if replayed.Code != http.StatusOK || createCalls.Load() != 1 {
		t.Fatalf("same idempotency key did not return prior review: %d %s", replayed.Code, replayed.Body)
	}

	// An edited provider connection invalidates the review before AgentHub can authorize a write.
	dir.items[0].BaseURL = "https://changed.example.test"
	execute := func(reviewID string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
		r.Header.Set(noteHeader, "Restart guest after maintenance")
		r.Header.Set(infraPowerReviewHeader, reviewID)
		r.SetPathValue("id", serverID)
		r.SetPathValue("guest", "guest-a")
		r.SetPathValue("verb", "reboot")
		w := httptest.NewRecorder()
		a.infraStatus(w, r, c)
		return w
	}
	stale := execute(prepared.Review.ID)
	if stale.Code != http.StatusConflict || driver.calls.Load() != 0 {
		t.Fatalf("stale endpoint reached provider: %d %s calls=%d", stale.Code, stale.Body, driver.calls.Load())
	}
	dir.items[0] = ep
	status.Store("passed")
	currentTicketSHA = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	changedTicket := execute(prepared.Review.ID)
	if changedTicket.Code != http.StatusConflict || !strings.Contains(changedTicket.Body.String(), "agenthub_review_stale") || driver.calls.Load() != 0 {
		t.Fatalf("changed AgentHub ticket reached provider: %d %s calls=%d", changedTicket.Code, changedTicket.Body, driver.calls.Load())
	}
	currentTicketSHA = powerReviewTicketSHA
	read := httptest.NewRequest(http.MethodGet, "/", nil)
	read.SetPathValue("id", serverID)
	read.SetPathValue("guest", "guest-a")
	read.SetPathValue("reviewID", prepared.Review.ID)
	view := httptest.NewRecorder()
	a.getInfraPowerReview(view, read, c)
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), `"agenthub_status":"passed"`) {
		t.Fatalf("ticket status %d %s", view.Code, view.Body)
	}
	allowed := execute(prepared.Review.ID)
	if allowed.Code != http.StatusOK || driver.calls.Load() != 1 {
		t.Fatalf("approved action %d %s calls=%d", allowed.Code, allowed.Body, driver.calls.Load())
	}
	replay := execute(prepared.Review.ID)
	if replay.Code != http.StatusConflict || driver.calls.Load() != 1 {
		t.Fatalf("review replay was not refused: %d %s calls=%d", replay.Code, replay.Body, driver.calls.Load())
	}
	var state string
	if err := a.db.Pool.QueryRow(ctx, `SELECT state FROM shippin_infra_power_reviews WHERE id=$1`, prepared.Review.ID).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("review terminal state %q err=%v", state, err)
	}
	var events int
	if err := a.db.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_infra_power_review_events WHERE review_id=$1`, prepared.Review.ID).Scan(&events); err != nil || events < 3 {
		t.Fatalf("missing append-only review events: count=%d err=%v", events, err)
	}

	status.Store("pending_review")
	secondRequest := prepare(uuid.NewString())
	if secondRequest.Code != http.StatusAccepted {
		t.Fatalf("second ticket request %d %s", secondRequest.Code, secondRequest.Body)
	}
	var second struct {
		Review servers.PowerReview `json:"review"`
	}
	if err := json.Unmarshal(secondRequest.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	denied := execute(second.Review.ID)
	if denied.Code != http.StatusConflict || !strings.Contains(denied.Body.String(), "agenthub_review_denied") || driver.calls.Load() != 1 {
		t.Fatalf("pending ticket was not denied: %d %s calls=%d", denied.Code, denied.Body, driver.calls.Load())
	}
	server.Close()
	outage := execute(second.Review.ID)
	if outage.Code != http.StatusBadGateway || !strings.Contains(outage.Body.String(), "agenthub_review_unavailable") || driver.calls.Load() != 1 {
		t.Fatalf("AgentHub outage reached provider: %d %s calls=%d", outage.Code, outage.Body, driver.calls.Load())
	}
}

func writePowerReviewTicket(w http.ResponseWriter, id, tenant, machine, command, actionSHA, status, ticketSHA string, fields map[string]string) {
	_, _ = w.Write([]byte(ticketJSON(id, tenant, machine, command, actionSHA, status, ticketSHA, fields)))
}
func writePowerReviewEnvelope(w http.ResponseWriter, id, tenant, machine, command, actionSHA, status, ticketSHA string, fields map[string]string) {
	_, _ = w.Write([]byte(`{"ticket":` + ticketJSON(id, tenant, machine, command, actionSHA, status, ticketSHA, fields) + `}`))
}
func ticketJSON(id, tenant, machine, command, actionSHA, status, ticketSHA string, fields map[string]string) string {
	b, _ := json.Marshal(fields)
	return `{"id":"` + id + `","tenant_id":"` + tenant + `","machine":"` + machine + `","command":` + string(mustJSON(command)) + `,"fields":` + string(b) + `,"sha256":"` + ticketSHA + `","status":"` + status + `","version":1}`
}
