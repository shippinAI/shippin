package api

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestModesLegacyShadowEnforce(t *testing.T) {
	for _, tc := range []struct {
		mode     string
		fallback bool
		user     string
		iam      bool
		want     int
		audit    string
	}{
		{"legacy", false, "sup", false, 403, "authz.denied"}, // staff roles mean nothing yet
		{"legacy", false, "iam", true, 204, ""},
		{"shadow", false, "sup", false, 403, "authz.shadow_mismatch"}, // old gate decides, mismatch logged
		{"shadow", false, "iam", true, 204, "authz.shadow_mismatch"},
		{"enforce", false, "sup", false, 204, ""},
		{"enforce", false, "iam", true, 403, "authz.denied"},
		{"enforce", true, "iam", true, 204, "authz.legacy_iam_owner"}, // logged fallback
	} {
		store := newFakeStore(tc.mode)
		store.settings.LegacyFallback = tc.fallback
		a := staffAPI(store, newFakeIdP().add("sup", "staff.support").add("iam"))
		if got := call(stubMux(a), "GET /v1/operator/tenants", tc.user, tc.iam); got != tc.want {
			t.Errorf("%+v: got %d", tc, got)
		}
		if tc.audit != "" && !slices.Contains(store.actions(), tc.audit) {
			t.Errorf("%+v: audit %v lacks %s", tc, store.actions(), tc.audit)
		}
	}
}

// The config override is the rollback switch: it beats the stored mode.
func TestConfigModeOverrideWins(t *testing.T) {
	a := staffAPI(newFakeStore("enforce"), newFakeIdP().add("sup", "staff.support"))
	a.cfg.Staff.Mode = "legacy"
	if got := call(stubMux(a), "GET /v1/operator/tenants", "sup", false); got != 403 {
		t.Fatalf("legacy override must ignore staff roles, got %d", got)
	}
}

func TestRouteWithoutPermissionStopsStartup(t *testing.T) {
	for _, perm := range []string{"", "tenants.everything", "mail.read"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("perm %q: mounting must panic", perm)
				}
			}()
			a := staffAPI(newFakeStore("enforce"), newFakeIdP())
			a.mountOperator(http.NewServeMux(), []opRoute{{"GET /v1/operator/x", perm, nil}})
		}()
	}
}

// /me and the gates agree: the effective permissions /me reports are exactly what the gates allow.
func TestMeMatchesGates(t *testing.T) {
	for _, mode := range []string{"legacy", "shadow", "enforce"} {
		for _, role := range []string{"staff.owner", "staff.support", "staff.auditor"} {
			a := staffAPI(newFakeStore(mode), newFakeIdP().add("u1", role))
			r := httptest.NewRequest("GET", "/v1/me", nil)
			perms := a.staffSummary(r, &caller{id: testID("u1")})["permissions"].([]string)
			mux := stubMux(a)
			for _, rt := range a.operatorRoutes() {
				if rt.perm == permPanel {
					continue
				}
				gate := call(mux, rt.pattern, "u1", false) == 204
				if gate != slices.Contains(perms, rt.perm) {
					t.Errorf("%s %s %s: gate=%v, /me=%v", mode, role, rt.pattern, gate, !gate)
				}
			}
		}
	}
}
