package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

func TestSparkInventoryUsesTenantAndToolPermissions(t *testing.T) {
	for _, tc := range []struct {
		name, op, tenant, fleet string
		grant                   bool
		want                    int
		visible                 bool
	}{
		{"owner", "op_superadmin", "t1", "t1", false, 200, true},
		{"non admin member", "", "t1", "t1", false, 403, false},
		{"support", "op_support", "t1", "t1", false, 403, false},
		{"other tenant", "op_superadmin", "t2", "t1", false, 200, false},
		{"no membership", "op_superadmin", "", "t1", false, 404, false},
		{"temporary grant", "op_superadmin", "t1", "t1", true, 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := gateAPI(tc.op)
			a.flow = &auth.Flow{}
			a.cfg.Sparks = config.Sparks{TenantID: tc.fleet, Nodes: []config.SparkNode{{ID: "spark-private"}}}
			a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
				c := &caller{id: testID("u1"), op: tc.op}
				if tc.tenant != "" {
					v := tenantView{Tenant: db.Tenant{ID: tc.tenant}, Roles: []string{"org_owner"}}
					if tc.grant {
						v.grants = []db.Grant{{ID: "grant"}}
					}
					c.tenants = []tenantView{v}
				}
				return c, true
			}
			mux := http.NewServeMux()
			a.Register(mux)
			path := tc.tenant
			if path == "" {
				path = "t1"
			}
			r := httptest.NewRequest("GET", "/v1/tenants/"+path+"/sparks", nil)
			r.Header.Set("Authorization", "Bearer test")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.want || strings.Contains(w.Body.String(), "spark-private") != tc.visible {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" && tc.want == 200 {
				t.Fatal("inventory must not be cached")
			}
		})
	}
}

func TestSparkInventoryRejectsAnonymousRequest(t *testing.T) {
	a := gateAPI("op_superadmin")
	a.flow = &auth.Flow{}
	mux := http.NewServeMux()
	a.Register(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/v1/tenants/t1/sparks", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}
