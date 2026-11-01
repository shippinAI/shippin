package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/jobs"
	"testing"
)

func TestWorkspaceActionReplaysDeletedWorkspace(t *testing.T) {
	a, d, tn := notifyAPI(t)
	a.jobs = jobs.NewStore(d.Pool)
	a.runner = jobs.NewRunner(a.jobs, jobs.Settings{})
	a.cfg.Jobs.IdempotencyHours = 1
	ws := provisioningWS(t, a, d, tn)
	if e := d.SetWorkspaceState(t.Context(), ws.ID, "running", "", ""); e != nil {
		t.Fatal(e)
	}

	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oidc/v1/userinfo":
			json.NewEncoder(w).Encode(map[string]string{"sub": "u", "email": "u@example.test"})
		case "/auth/v1/memberships/me/_search":
			memberships := []auth.Membership{}
			if r.Header.Get("Authorization") == "Bearer fixture-member" {
				memberships = append(memberships, auth.Membership{OrgID: tn.IdpOrgID, Roles: []string{"ORG_OWNER"}})
			}
			json.NewEncoder(w).Encode(map[string]any{"result": memberships})
		default:
			w.WriteHeader(404)
		}
	}))
	defer idp.Close()
	a.cfg.IDP.Issuer = idp.URL
	a.flow = auth.NewFlow(a.cfg, nil)
	a.flow.Client = idp.Client()
	token := "fixture-member"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tenants/{tenantId}/workspaces/{workspaceId}/actions/{action}", a.flow.Require(a.workspaceAction))
	key := newUUID()
	call := func() *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/tenants/"+tn.ID+"/workspaces/"+ws.ID+"/actions/delete", nil)
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("Authorization", "Bearer "+token)
		mux.ServeHTTP(rr, req)
		return rr
	}
	first := call()
	if first.Code != 202 {
		t.Fatalf("first %d %s", first.Code, first.Body.String())
	}
	if e := d.SetWorkspaceState(t.Context(), ws.ID, "deleted", "", ""); e != nil {
		t.Fatal(e)
	}
	if _, e := d.Pool.Exec(t.Context(), `UPDATE jobs SET status='succeeded' WHERE target_id=$1`, ws.ID); e != nil {
		t.Fatal(e)
	}
	again := call()
	if again.Code != 202 || again.Body.String() != first.Body.String() || again.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("deleted replay %d %s", again.Code, again.Body.String())
	}
	token = "fixture-removed"
	if denied := call(); denied.Code != 404 {
		t.Fatalf("removed tenant visibility exposed receipt: %d", denied.Code)
	}
}
