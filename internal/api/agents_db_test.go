package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agents"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

// TestAgentIdentities: the agent is an IdP service user in the tenant's own org, delegated by the
// creating owner, audited before the IdP write, capped per tenant, unique by name, and no IdP
// write happens on a refusal.
func TestAgentIdentities(t *testing.T) {
	a, d, tn := notifyAPI(t)
	a.cfg.Agents = config.Agents{Enabled: true, MaxPerTenant: 2, UserPrefix: "agent-"}
	var idpCalls []string
	fail := false
	a.agents = agentDeps{store: &agents.Store{Pool: d.Pool}, idp: func(_ context.Context, org, user, name, _ string) (string, error) {
		idpCalls = append(idpCalls, org+" "+user+" "+name)
		if fail {
			return "", errors.New("idp down")
		}
		return "idp-" + uuid.NewString(), nil
	}}
	mgr := []string{"org_owner", "org_admin"}
	me := &caller{id: &auth.Identity{Sub: "sub-owner"}, managers: mgr, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_owner"}}}}
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		a.doCreateAgent(w, httptest.NewRequest("POST", "/x", strings.NewReader(body)), me, &me.tenants[0])
		return w
	}
	for body, want := range map[string]int{`{"name":""}`: 400, `{"name":"a\nb"}`: 400, `nope`: 400,
		`{"name":"x","workspace_id":"` + uuid.NewString() + `"}`: 409} {
		if w := post(body); w.Code != want {
			t.Errorf("%s: %d, want %d", body, w.Code, want)
		}
	}
	if len(idpCalls) != 0 {
		t.Fatalf("refused requests must not reach the IdP: %v", idpCalls)
	}
	w := post(`{"name":"Build bot"}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"delegating_user_id":"sub-owner"`) || strings.Contains(w.Body.String(), "token") {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if len(idpCalls) != 1 || !strings.HasPrefix(idpCalls[0], tn.IdpOrgID+" agent-") {
		t.Fatalf("IdP call must target the tenant's org with the prefix: %v", idpCalls)
	}
	if w := post(`{"name":"Build bot"}`); w.Code != 409 {
		t.Fatalf("duplicate name: %d", w.Code)
	}
	fail = true
	if w := post(`{"name":"Second"}`); w.Code != 502 {
		t.Fatalf("idp failure: %d", w.Code)
	}
	fail = false
	if w := post(`{"name":"Second"}`); w.Code != 201 {
		t.Fatalf("second: %d", w.Code)
	}
	if w := post(`{"name":"Third"}`); w.Code != 409 || !strings.Contains(w.Body.String(), "limit") {
		t.Fatalf("cap: %d %s", w.Code, w.Body)
	}
	got, err := (&agents.Store{Pool: d.Pool}).List(context.Background(), tn.ID)
	if err != nil || len(got) != 2 || got[0].Name != "Second" {
		t.Fatalf("list: %+v %v", got, err)
	}
	var audited int
	_ = d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action='agent.create' AND tenant_id=$1`, tn.ID).Scan(&audited)
	if audited != 3 { // two created + the attempt the IdP refused (audit comes first)
		t.Fatalf("audit entries: %d", audited)
	}
	if !me.canDecide(&me.tenants[0]) {
		t.Fatal("the owner may create agents")
	}
	viewer := &caller{id: &auth.Identity{Sub: "sub-dev"}, managers: mgr, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_member"}}}}
	if viewer.canDecide(&viewer.tenants[0]) {
		t.Fatal("a plain member must not create agents")
	}
}
