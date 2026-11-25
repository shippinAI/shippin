package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/runtime/devspace"
	"shippin.cloud/console/internal/services/jobs"
)

// lifecycleAPI serves the generated actions behind the real session and member gate. The token
// "fixture-member" is an owner of tn; any other token is signed in and belongs to nothing.
func lifecycleAPI(t *testing.T) (a *API, d *db.DB, tn db.Tenant, call func(method, path, key, token string) *httptest.ResponseRecorder) {
	t.Helper()
	a, d, tn = notifyAPI(t)
	a.jobs = jobs.NewStore(d.Pool)
	a.runner.Store(jobs.NewRunner(a.jobs, jobs.Settings{}))
	a.cfg.Jobs.IdempotencyHours = 1
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
	t.Cleanup(idp.Close)
	a.cfg.IDP.Issuer = idp.URL
	a.flow = auth.NewFlow(a.cfg, nil)
	a.flow.Client = idp.Client()
	mux := http.NewServeMux()
	a.mountActions(mux)
	call = func(method, path, key, token string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/v1/tenants/"+tn.ID+"/runtime/devspaces/"+path, nil)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		mux.ServeHTTP(rr, req)
		return rr
	}
	return a, d, tn, call
}

// A lifecycle action is accepted once: 202 with the job to follow. A repeat with the same key gets
// the first answer and starts nothing; the same key for another action is refused; a second action
// while the first is in progress is refused. The acceptance is audited as the action itself.
func TestDevspaceActionAcceptReplayAndConflict(t *testing.T) {
	a, d, tn, call := lifecycleAPI(t)
	ws := provisioningWS(t, a, d, tn)
	if err := devspace.On(d.Pool).SetState(t.Context(), ws.ID, "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	key := newUUID()
	first := call("POST", ws.ID+"/start", key, "fixture-member")
	if first.Code != 202 || !strings.HasPrefix(first.Header().Get("Location"), "/v1/jobs/") {
		t.Fatalf("acceptance: %d %s", first.Code, first.Body.String())
	}
	var accepted struct {
		ID, State string
		Job       struct {
			ID, Kind, Status string
			Target           struct{ Type, ID string }
		}
	}
	if err := json.Unmarshal(first.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.ID != ws.ID || accepted.State != "starting" || accepted.Job.ID == "" || accepted.Job.Kind != jobs.KindWorkspaceAction ||
		accepted.Job.Target.ID != ws.ID || first.Header().Get("Location") != "/v1/jobs/"+accepted.Job.ID {
		t.Fatalf("acceptance body: %s", first.Body.String())
	}
	replay := call("POST", ws.ID+"/start", key, "fixture-member")
	if replay.Code != 202 || replay.Body.String() != first.Body.String() || replay.Header().Get("Location") != first.Header().Get("Location") || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay changed the receipt: %d %s", replay.Code, replay.Body.String())
	}
	if got := call("POST", ws.ID+"/stop", key, "fixture-member"); got.Code != 422 {
		t.Fatalf("key reused for another action: %d", got.Code)
	}
	if got := call("POST", ws.ID+"/stop", newUUID(), "fixture-member"); got.Code != 409 {
		t.Fatalf("second action while the first is in progress: %d", got.Code)
	}
	var n int
	if err := d.Pool.QueryRow(t.Context(), `SELECT count(*) FROM jobs WHERE target_id=$1`, ws.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("jobs=%d err=%v", n, err)
	}
	if err := d.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE action='shippin.runtime.devspace.start' AND workspace_id::text=$1 AND actor='u'`, ws.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit entries for the accepted start=%d err=%v, want one", n, err)
	}
	if got := call("POST", ws.ID+"/start", newUUID(), "fixture-stranger"); got.Code != 404 {
		t.Fatalf("someone outside the organization: %d", got.Code)
	}
	if got := call("POST", newUUID()+"/start", newUUID(), "fixture-member"); got.Code != 404 {
		t.Fatalf("a space that does not exist: %d", got.Code)
	}
}

// The answer to an accepted delete is still replayed after the space is gone, and never to someone
// who no longer sees the organization.
func TestDevspaceDeleteReplaysAfterDeletion(t *testing.T) {
	a, d, tn, call := lifecycleAPI(t)
	ws := provisioningWS(t, a, d, tn)
	if e := devspace.On(d.Pool).SetState(t.Context(), ws.ID, "running", "", ""); e != nil {
		t.Fatal(e)
	}
	key := newUUID()
	first := call("DELETE", ws.ID, key, "fixture-member")
	if first.Code != 202 {
		t.Fatalf("first %d %s", first.Code, first.Body.String())
	}
	if e := devspace.On(d.Pool).SetState(t.Context(), ws.ID, "deleted", "", ""); e != nil {
		t.Fatal(e)
	}
	if _, e := d.Pool.Exec(t.Context(), `UPDATE jobs SET status='succeeded' WHERE target_id=$1`, ws.ID); e != nil {
		t.Fatal(e)
	}
	again := call("DELETE", ws.ID, key, "fixture-member")
	if again.Code != 202 || again.Body.String() != first.Body.String() || again.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("deleted replay %d %s", again.Code, again.Body.String())
	}
	if denied := call("DELETE", ws.ID, key, "fixture-removed"); denied.Code != 404 {
		t.Fatalf("removed tenant visibility exposed receipt: %d", denied.Code)
	}
}

// With durable work turned off nothing is accepted and the space is left as it was.
func TestDevspaceActionNeedsTheQueue(t *testing.T) {
	a, d, tn, call := lifecycleAPI(t)
	a.runner.Store(nil)
	ws := provisioningWS(t, a, d, tn)
	if e := devspace.On(d.Pool).SetState(t.Context(), ws.ID, "stopped", "", ""); e != nil {
		t.Fatal(e)
	}
	if got := call("POST", ws.ID+"/start", newUUID(), "fixture-member"); got.Code != 503 {
		t.Fatalf("accepted without a queue: %d %s", got.Code, got.Body.String())
	}
	row, _ := devspace.On(d.Pool).ByID(t.Context(), ws.ID)
	var n int
	if e := d.Pool.QueryRow(t.Context(), `SELECT count(*) FROM jobs WHERE target_id=$1`, ws.ID).Scan(&n); e != nil || n != 0 || row.State != "stopped" {
		t.Fatalf("state=%s jobs=%d err=%v", row.State, n, e)
	}
}
