package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/orgs"
)

// orgTestAPI: real DB, orgs on, the caller swappable per request (who).
func orgTestAPI(t *testing.T) (*API, db.Tenant, *httptest.Server, *string) {
	t.Helper()
	a, _, tn := notifyAPI(t)
	a.cfg.Orgs = config.Orgs{Enabled: true, OperationsSlug: "operations", OperationsName: "Operations",
		BootstrapAdminSub: "sub-operator", SupportGrantMaxSeconds: 3600}
	who := new(string)
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		c := &caller{id: &auth.Identity{Sub: *who}}
		if *who == "sub-operator" || *who == "sub-tenant" {
			c.tenants = []tenantView{{Tenant: tn, Roles: []string{"org_owner"}}}
		}
		return c, true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs", a.listOrgs)
	mux.HandleFunc("GET /v1/ops/servers", a.opsGate(false, a.opsServers))
	mux.HandleFunc("POST /v1/ops/support-grants", a.opsGate(true, a.grantSupport))
	mux.HandleFunc("POST /v1/ops/support-grants/{id}/revoke", a.opsGate(true, a.revokeSupport))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return a, tn, srv, who
}

func types(out map[string]any) []string {
	items, _ := out["items"].([]any)
	r := []string{}
	for _, it := range items {
		m := it.(map[string]any)
		r = append(r, m["type"].(string))
	}
	return r
}

func TestOrgSwitcherAndStaffGate(t *testing.T) {
	a, _, srv, who := orgTestAPI(t)
	*who = "sub-tenant" // tenant owner, no staff role
	if c, out, _ := srvCall(t, srv, "GET", "/v1/orgs", "", ""); c != 200 || len(types(out)) != 1 || types(out)[0] != "customer" {
		t.Fatal("tenant orgs", c, out)
	}
	if c, _, _ := srvCall(t, srv, "GET", "/v1/ops/servers", "", ""); c != 403 {
		t.Fatal("tenant user reached Operations servers", c)
	}
	*who = "sub-operator" // bootstrap: becomes ops_admin, sees Operations + his Builder org
	if c, out, _ := srvCall(t, srv, "GET", "/v1/orgs", "", ""); c != 200 || len(types(out)) != 2 || types(out)[0] != "operations" {
		t.Fatal("operator orgs", c, out)
	}
	s := a.orgStore()
	if err := s.GrantStaffRole(t.Context(), "sub-oncall", orgs.RoleOncall, "sub-operator", nil); err != nil {
		t.Fatal(err)
	}
	*who = "sub-oncall" // staff with no devspace at all
	if c, out, _ := srvCall(t, srv, "GET", "/v1/ops/servers", "", ""); c != 200 || out["can_admin"] != false {
		t.Fatal("staff without devspace refused", c, out)
	}
	if c, out, _ := srvCall(t, srv, "GET", "/v1/orgs", "", ""); c != 200 || len(types(out)) != 1 || types(out)[0] != "operations" {
		t.Fatal("oncall orgs", c, out)
	}
	if c, _, _ := srvCall(t, srv, "POST", "/v1/ops/support-grants", `{}`, ""); c != 403 {
		t.Fatal("oncall issued a support grant", c)
	}
	*who = "sub-nobody"
	if c, _, _ := srvCall(t, srv, "GET", "/v1/ops/servers", "", ""); c != 403 {
		t.Fatal("stranger reached Operations", c)
	}
	a.cfg.Orgs.Enabled = false
	if c, _, _ := srvCall(t, srv, "GET", "/v1/orgs", "", ""); c != 404 {
		t.Fatal("disabled orgs answered", c)
	}
}

func TestSupportGrantAuditedAndCapped(t *testing.T) {
	a, tn, srv, who := orgTestAPI(t)
	*who = "sub-operator"
	c0, out, _ := srvCall(t, srv, "GET", "/v1/orgs", "", "")
	if c0 != 200 || len(types(out)) != 2 {
		t.Fatal("orgs", c0, out)
	}
	builder := out["items"].([]any)[1].(map[string]any)["id"].(string)
	body := func(ttl string) string {
		return `{"staff_sub":"sub-support","org_id":"` + builder + `","reason":"customer ticket 42: build fails","ttl_seconds":` + ttl + `}`
	}
	if c, _, _ := srvCall(t, srv, "POST", "/v1/ops/support-grants", body("7200"), ""); c != 400 {
		t.Fatal("grant over the configured max accepted", c)
	}
	c, g, _ := srvCall(t, srv, "POST", "/v1/ops/support-grants", body("1800"), "")
	if c != 201 || g["expires_at"] == nil {
		t.Fatal(c, g)
	}
	if c, r, _ := srvCall(t, srv, "POST", "/v1/ops/support-grants/"+g["id"].(string)+"/revoke", "", ""); c != 200 || r["revoked_at"] == nil {
		t.Fatal("revoke", c, r)
	}
	var n int
	if err := a.db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE action IN ('ops.support_grant','ops.support_revoke') AND actor='sub-operator'`).Scan(&n); err != nil || n != 3 {
		t.Fatal("audit rows", n, err) // two grant attempts (audit-first) + one revoke
	}
	_ = tn
}
