package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

// serverTestAPI: a real DB, servers enabled from config values, and a mux serving the CP-P1
// routes the way Register does (minus the session middleware; the caller is injected).
func serverTestAPI(t *testing.T, role string) (*API, *db.DB, db.Tenant, *httptest.Server) {
	t.Helper()
	a, d, tn := notifyAPI(t)
	a.cfg.Servers = config.Servers{Enabled: true, TokenTTLSeconds: 600, HeartbeatSeconds: 1, MissedBeats: 1,
		CredentialTTLSeconds: 30, MaxPerTenant: 3}
	me := &caller{id: &auth.Identity{Sub: "sub-" + role}, managers: []string{"org_owner", "org_admin"},
		tenants: []tenantView{{Tenant: tn, Roles: []string{role}}}}
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return me, true }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/tenants/{tenantId}/servers", a.serverAdmin(a.listServers))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/servers", a.serverAdmin(a.createServer))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/servers/{serverId}/enroll-token", a.serverAdmin(a.issueServerToken))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/servers/{serverId}/revoke", a.serverAdmin(a.revokeServer))
	mux.HandleFunc("POST /v1/servers/enroll", a.enrollServer)
	mux.HandleFunc("POST /v1/servers/heartbeat", a.serverHeartbeat)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return a, d, tn, srv
}

// call sends method path body (bearer optional) and decodes a JSON object reply.
func srvCall(t *testing.T, srv *httptest.Server, method, path, body, bearer string) (int, map[string]any, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString()) // as the SPA sends; must never cache a token
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out, res.Header
}

// stateOf returns the listed state of server id.
func stateOf(t *testing.T, srv *httptest.Server, tid, id string) string {
	_, out, _ := srvCall(t, srv, "GET", "/v1/tenants/"+tid+"/servers", "", "")
	items, _ := out["items"].([]any)
	for _, it := range items {
		if m := it.(map[string]any); m["id"] == id {
			return m["state"].(string)
		}
	}
	return ""
}
