package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/identity"
)

func TestOrganizationWorkspaceRoutesEnforceMembershipAndManagement(t *testing.T) {
	for _, role := range []string{"org_owner", "org_member"} {
		t.Run(role, func(t *testing.T) {
			a, database, tenant, _ := serverTestAPI(t, role)
			user := uuid.NewString()
			if _, err := database.Pool.Exec(t.Context(), `INSERT INTO identity_users(id,email,name,role,email_verified) VALUES($1,$2,'Customer','member',true)`, user, user+"@example.invalid"); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Pool.Exec(t.Context(), `INSERT INTO identity_orgs(id,name) VALUES($1,'Customer')`, tenant.IdpOrgID); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Pool.Exec(t.Context(), `INSERT INTO identity_members(org_id,user_id,role) VALUES($1,$2,$3)`, tenant.IdpOrgID, user, strings.TrimPrefix(role, "org_")); err != nil {
				t.Fatal(err)
			}
			store, err := auth.NewStore(database, strings.Repeat("ab", 32))
			if err != nil {
				t.Fatal(err)
			}
			a.flow = auth.NewFlow(a.cfg, store)
			a.directory = identity.New(database.Pool, a.cfg, nil, nil)
			signed := httptest.NewRecorder()
			if _, err := a.flow.StartLocal(signed, httptest.NewRequest("POST", "/v1/session/password", nil), user); err != nil {
				t.Fatal(err)
			}
			cookie := signed.Result().Cookies()[0]
			mux := http.NewServeMux()
			a.projectRoutes(mux)
			call := func(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				if authenticated {
					r.AddCookie(cookie)
				}
				r.Header.Set("Idempotency-Key", uuid.NewString())
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				return w
			}
			base := "/v1/tenants/" + tenant.ID + "/organization-workspaces"
			if w := call("GET", base, "", false); w.Code != 401 {
				t.Fatal("anonymous read", w.Code)
			}
			if w := call("GET", base, "", true); w.Code != 200 {
				t.Fatal("member list", w.Code, w.Body.String())
			}
			if w := call("GET", "/v1/tenants/"+uuid.NewString()+"/organization-workspaces", "", true); w.Code != 404 {
				t.Fatal("foreign tenant read", w.Code)
			}
			w := call("POST", base, `{"name":"Product"}`, true)
			if role == "org_member" {
				if w.Code != 403 {
					t.Fatal("member create", w.Code)
				}
				for _, method := range []string{"PATCH", "DELETE"} {
					if w := call(method, base+"/"+uuid.NewString(), `{"name":"Changed"}`, true); w.Code != 403 {
						t.Fatal("member write", method, w.Code)
					}
				}
				return
			}
			if w.Code != 201 {
				t.Fatal("owner create", w.Code, w.Body.String())
			}
			var space struct{ ID string }
			if err := json.Unmarshal(w.Body.Bytes(), &space); err != nil {
				t.Fatal(err)
			}
			if w := call("POST", base, `{"name":"Product"}`, true); w.Code != 409 {
				t.Fatal("duplicate", w.Code)
			}
			if w := call("POST", base, `{"name":"Bad","tenant_id":"injected"}`, true); w.Code != 400 {
				t.Fatal("unknown field", w.Code)
			}
			if w := call("PATCH", base+"/"+space.ID, `{"name":"Renamed","description":"Persistent"}`, true); w.Code != 200 {
				t.Fatal("edit", w.Code, w.Body.String())
			}
			projectBase := "/v1/tenants/" + tenant.ID + "/projects"
			if w := call("POST", projectBase, `{"name":"test-project","workspace_id":"`+space.ID+`"}`, true); w.Code != 201 {
				t.Fatal("project create", w.Code, w.Body.String())
			}
			if w := call("GET", base+"/"+space.ID+"/projects", "", true); w.Code != 200 || !strings.Contains(w.Body.String(), "test-project") {
				t.Fatal("project list", w.Code, w.Body.String())
			}
			if w := call("DELETE", base+"/"+space.ID, "", true); w.Code != 409 {
				t.Fatal("occupied workspace", w.Code)
			}
		})
	}
}
