package api

import (
	"net/http/httptest"
	"shippin.cloud/console/internal/jobs"
	"testing"
)

func TestWorkspaceActionReplayAndConflict(t *testing.T) {
	a, d, tn := notifyAPI(t)
	a.jobs = jobs.NewStore(d.Pool)
	a.runner = jobs.NewRunner(a.jobs, jobs.Settings{})
	a.cfg.Jobs.IdempotencyHours = 1
	ws := provisioningWS(t, a, d, tn)
	if err := d.SetWorkspaceState(t.Context(), ws.ID, "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	call := func(action, key string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/actions/"+action, nil)
		req.Header.Set("Idempotency-Key", key)
		a.runAction(rr, req, &tenantView{Tenant: tn}, *ws, action, "u")
		return rr
	}
	key := newUUID()
	first := call("start", key)
	if first.Code != 202 {
		t.Fatalf("%d %s", first.Code, first.Body.String())
	}
	replay := call("start", key)
	if replay.Code != 202 || replay.Body.String() != first.Body.String() || replay.Header().Get("Location") != first.Header().Get("Location") || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatal("replay changed receipt")
	}
	if got := call("stop", key); got.Code != 422 {
		t.Fatalf("key conflict %d", got.Code)
	}
	if got := call("stop", newUUID()); got.Code != 409 {
		t.Fatalf("concurrent conflict %d", got.Code)
	}
	var n int
	if err := d.Pool.QueryRow(t.Context(), `SELECT count(*) FROM jobs WHERE target_id=$1`, ws.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("jobs=%d err=%v", n, err)
	}
}
