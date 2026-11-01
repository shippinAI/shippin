package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"testing"
)

func TestLifecycleRevokedGrantDoesNotDispatch(t *testing.T) {
	a, d, tn := notifyAPI(t)
	a.jobs = jobs.NewStore(d.Pool)
	ws := provisioningWS(t, a, d, tn)
	if err := d.SetWorkspaceState(t.Context(), ws.ID, "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	g := db.Grant{ID: newUUID(), TenantID: tn.ID, WorkspaceID: ws.ID, Kind: "request", Requester: "u", RequesterName: "test", Reason: "fixture", DurationS: 3600, Status: "approved"}
	if err := d.CreateGrant(t.Context(), g); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(workspaceActionIn{Action: "start", Grant: g.ID})
	j, err := a.jobs.EnqueueAction(t.Context(), jobs.Job{TenantID: tn.ID, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 3, Payload: payload}, "starting")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := d.TransitionGrant(t.Context(), g.ID, "revoke", "owner", "Owner"); err != nil || !ok {
		t.Fatalf("revoke: %v %v", ok, err)
	}
	j.LeaseOwner = "worker"
	if _, err := d.Pool.Exec(t.Context(), `UPDATE jobs SET status='running',lease_owner='worker',lease_until=now()+interval '1 minute' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("revoked grant reached provider")
		w.WriteHeader(500)
	}))
	defer srv.Close()
	a.host = hostclient.New(srv.URL)
	if _, err := a.runWorkspaceActionJob(t.Context(), j, func(string) {}); !jobs.IsPermanent(err) {
		t.Fatalf("must fail permanently: %v", err)
	}
	row, _ := d.WorkspaceByID(t.Context(), ws.ID)
	if row.State != "failed" {
		t.Fatal(row.State)
	}
}
