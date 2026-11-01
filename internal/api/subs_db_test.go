package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/subs"
)

// subsAPI: notifyAPI with AI activation on and a fake hostd that records every call.
func subsAPI(t *testing.T) (*API, *db.DB, db.Tenant, func() []string) {
	a, d, tn := notifyAPI(t)
	a.subs = &subs.Store{Pool: d.Pool}
	a.cfg.AI = config.AI{Enabled: true, IdleMinutes: 15, TouchSeconds: 60, Providers: []config.AIProvider{
		{ID: "claude", Label: "Claude Code", LoginCommand: "claude auth login"},
		{ID: "codex", Label: "Codex", LoginCommand: "codex login --device-auth"}}}
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path+" "+body["host"]+body["provider"]+body["account"])
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/ai-status") {
			_, _ = w.Write([]byte(`{"version":2,"checked_at":"` + time.Now().UTC().Format(time.RFC3339) + `","providers":[{"provider":"claude","installed":true,"signed_in":true,"auth_known":true},{"provider":"codex","installed":true,"signed_in":false,"auth_known":true}]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"IPv4": "10.1.2.3"})
	}))
	t.Cleanup(srv.Close)
	a.host = hostclient.New(srv.URL)
	return a, d, tn, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string{}, calls...) }
}

func subsCall(a *API, c *caller, h http.HandlerFunc, method, tenant, provider string) *httptest.ResponseRecorder {
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return c, true }
	r := httptest.NewRequest(method, "/x", nil)
	r.SetPathValue("tenantId", tenant)
	r.SetPathValue("provider", provider)
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func hasCall(calls []string, parts ...string) bool {
	for _, c := range calls {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(c, p)
		}
		if ok {
			return true
		}
	}
	return false
}

func TestSubscriptionActivationFlow(t *testing.T) {
	a, d, tn, calls := subsAPI(t)
	ctx := context.Background()
	me := &caller{id: &auth.Identity{Sub: "sub-" + uuid.NewString(), Email: "me@acme.test"},
		tenants: []tenantView{{Tenant: tn, Roles: []string{"org_member"}}}}
	if w := subsCall(a, me, a.activateSubscription, "POST", tn.ID, "nope"); w.Code != 404 {
		t.Fatalf("unknown provider: %d", w.Code)
	}
	grantee := &caller{id: me.id, tenants: []tenantView{{Tenant: tn, grants: []db.Grant{{}}}}}
	if w := subsCall(a, grantee, a.activateSubscription, "POST", tn.ID, "claude"); w.Code != 403 {
		t.Fatalf("grant holders never activate in a customer tenant: %d", w.Code)
	}
	w := subsCall(a, me, a.activateSubscription, "POST", tn.ID, "claude")
	var out activateOut
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 202 || out.LoginCommand != "claude auth login" || !strings.HasPrefix(out.TerminalURL, "https://u-") ||
		strings.Contains(strings.ToLower(w.Body.String()), "token") {
		t.Fatalf("activate: %d %s", w.Code, w.Body)
	}
	acct := out.Home.AccountID
	for i := 0; i < 200; i++ {
		if h, _ := a.subs.HomeFor(ctx, tn.ID, acct, ""); h != nil && h.State == "ready" && h.UtilityState == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !hasCall(calls(), "POST /tenants/"+tn.ID+"/utilities/"+acct+"/start", out.Home.UtilityHost) {
		t.Fatalf("terminal not started: %v", calls())
	}
	if w := subsCall(a, me, a.checkSubscription, "POST", tn.ID, "claude"); w.Code != 200 {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
	cs, _ := a.subs.Connections(ctx, acct)
	st := map[string]string{}
	for _, c := range cs {
		st[c.ProviderID] = c.Status
	}
	if st["claude"] != subs.Connected || st["codex"] != subs.NotConnected {
		t.Fatalf("statuses %v", st)
	}
	if !a.utilityOwner(ctx, me, out.Home.UtilityHost) {
		t.Fatal("owner opens their terminal")
	}
	admin := &caller{id: &auth.Identity{Sub: "sub-admin-" + uuid.NewString()}, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_owner"}}}}
	_, _ = d.UpsertAccount(ctx, admin.id.Sub, "")
	if a.utilityOwner(ctx, admin, out.Home.UtilityHost) || a.utilityOwner(ctx, me, "u-000000000000.ws.example.test") {
		t.Fatal("nobody else, not even a tenant owner, opens a user's AI terminal")
	}
	mine := a.hostReq(&tenantView{Tenant: tn}, db.Workspace{CreatedBy: me.id.Sub})
	theirs := a.hostReq(&tenantView{Tenant: tn}, db.Workspace{CreatedBy: admin.id.Sub})
	if mine.AIHome != "" || theirs.AIHome != "" || hasCall(calls(), "/ai-home") {
		t.Fatalf("provider credentials must stay off shared workspaces: %q %q", mine.AIHome, theirs.AIHome)
	}
	if w := subsCall(a, me, a.disconnectSubscription, "DELETE", tn.ID, "claude"); w.Code != 200 ||
		!hasCall(calls(), "/utilities/"+acct+"/logout", "claude") {
		t.Fatalf("disconnect: %d %v", w.Code, calls())
	}
	for _, state := range []string{"running", "starting", "error"} {
		_, err := d.Pool.Exec(ctx, `UPDATE ai_homes SET utility_state=$2, utility_seen=now()-interval '1 hour' WHERE account_id=$1`, acct, state)
		if err != nil {
			t.Fatal(err)
		}
		if state == "error" {
			if err := a.subs.SetUtility(ctx, out.Home.ID, "error", true); err != nil {
				t.Fatal(err)
			}
		}
		a.stopIdleTerminals(ctx)
		if h, _ := a.subs.HomeFor(ctx, tn.ID, acct, ""); h.UtilityState != "stopped" || !hasCall(calls(), "/utilities/"+acct+"/stop") {
			t.Fatalf("idle %s terminal not stopped: %+v", state, h)
		}
	}
	if w := subsCall(a, me, a.mySubscriptions, "GET", "", ""); w.Code != 200 || strings.Contains(w.Body.String(), "token") ||
		!strings.Contains(w.Body.String(), `"disconnected"`) {
		t.Fatalf("me: %d %s", w.Code, w.Body)
	}
}
