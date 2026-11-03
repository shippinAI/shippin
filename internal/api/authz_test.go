package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
)

func gateAPI(op string) *API {
	return &API{cfg: testCfg(), staffDB: newFakeStore("legacy"), staffIdP: newFakeIdP(),
		resolveCaller: func(http.ResponseWriter, *http.Request) (*caller, bool) {
			return &caller{id: &auth.Identity{Sub: "u1"}, op: op}, true
		}}
}

var pathVars = strings.NewReplacer("{tenantId}", "t1", "{workspaceId}", "w1", "{action}", "delete",
	"{templateId}", "x", "{grantId}", "g1", "{id}", "x", "{plan}", "free", "{service}", "dns", "{layer}", "system", "{key}", "k",
	"{userId}", "u9", "{permKey}", "catalog.plans.read")

// Every provider-panel route (including access requests, break-glass and the audit log) must
// answer 403 to anyone who isn't the platform admin role, before any handler code runs. The
// API has no DB here, so a leak past the gate would panic.
func TestOperatorRoutesRequirePlatformAdmin(t *testing.T) {
	for _, op := range []string{"", "op_support", "op_admin", "org_owner"} {
		a := gateAPI(op)
		mux := http.NewServeMux()
		a.registerOperator(mux)
		for _, rt := range a.operatorRoutes() {
			method, path, _ := strings.Cut(rt.pattern, " ")
			req := httptest.NewRequest(method, pathVars.Replace(path), strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("op=%q %s: got %d, want 403", op, rt.pattern, rec.Code)
			}
		}
	}
}

func TestOperatorRoutesNeedAToken(t *testing.T) {
	a := gateAPI("op_superadmin")
	a.flow = &auth.Flow{}
	mux := http.NewServeMux()
	a.registerOperator(mux)
	for _, rt := range a.operatorRoutes() {
		method, path, _ := strings.Cut(rt.pattern, " ")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, pathVars.Replace(path), nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without token: got %d, want 401", rt.pattern, rec.Code)
		}
	}
}

func TestAccessRoutesAreGated(t *testing.T) {
	want := []string{"GET /v1/operator/access-requests", "POST /v1/operator/access-requests",
		"POST /v1/operator/access-requests/{grantId}/end", "POST /v1/operator/break-glass", "GET /v1/operator/audit"}
	have := map[string]bool{}
	for _, rt := range gateAPI("").operatorRoutes() {
		have[rt.pattern] = true
	}
	for _, p := range want {
		if !have[p] {
			t.Errorf("%s is not mounted behind the platform admin gate", p)
		}
	}
}

func TestPlatformAdminRoleFromConfig(t *testing.T) {
	a := &API{cfg: testCfg()}
	if !a.isPlatformAdmin(&caller{op: "op_superadmin"}) || a.isPlatformAdmin(&caller{op: "op_support"}) {
		t.Fatal("only the configured role is platform admin")
	}
	a.cfg.Operator.Role = "op_admin"
	if a.isPlatformAdmin(&caller{op: "op_superadmin"}) || !a.isPlatformAdmin(&caller{op: "op_admin"}) {
		t.Fatal("configured role must be honoured exactly")
	}
	a.cfg.Operator.Role = ""
	if a.isPlatformAdmin(&caller{op: ""}) {
		t.Fatal("an empty role must never match")
	}
}

func TestRoleMappingFromConfig(t *testing.T) {
	a := &API{cfg: testCfg()}
	if a.opRole([]string{"IAM_OWNER"}) != "op_superadmin" || a.opRole([]string{"IAM_OWNER_VIEWER"}) != "op_support" ||
		a.opRole([]string{"IAM_ORG_MANAGER", "IAM_OWNER"}) != "op_superadmin" {
		t.Fatal("unexpected IAM role mapping")
	}
	if a.orgRole("ORG_OWNER") != "org_owner" || a.orgRole("ORG_USER_MANAGER") != "org_admin" || a.orgRole("ORG_OWNER_VIEWER") != "org_owner_viewer" {
		t.Fatal("unexpected org role mapping")
	}
	a.cfg.Roles.Org[0].To = "org_boss"
	if a.orgRole("ORG_OWNER") != "org_boss" {
		t.Fatal("org roles must come from config")
	}
}
