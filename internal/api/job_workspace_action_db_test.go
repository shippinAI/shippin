package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"testing"
)

func TestLifecycleDispatchAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, action, phase string
		partial             bool
		want                string
		posts               int
	}{
		{"start", "start", "", false, "running", 1},
		{"recovered start", "start", "dispatched", false, "running", 0},
		{"uncertain restart", "restart", "dispatched", false, "failed", 0},
		{"uncertain delete", "delete", "dispatched", false, "failed", 0},
		{"partial observation", "start", "acknowledged", true, "failed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, d, tn := notifyAPI(t)
			ctx := context.Background()
			a.jobs = jobs.NewStore(d.Pool)
			ws := provisioningWS(t, a, d, tn)
			if err := d.SetWorkspaceState(ctx, ws.ID, "stopped", "", ""); err != nil {
				t.Fatal(err)
			}
			payload, _ := json.Marshal(workspaceActionIn{Action: tc.action, Phase: tc.phase})
			j, err := a.jobs.EnqueueAction(ctx, jobs.Job{TenantID: tn.ID, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 3, Payload: payload}, actionState[tc.action][0])
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.Pool.Exec(ctx, `UPDATE jobs SET status='running',lease_owner='test',lease_until=now()+interval '1 minute' WHERE id=$1`, j.ID)
			if err != nil {
				t.Fatal(err)
			}
			j.LeaseOwner = "test"
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					w.Write([]byte(`{}`))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"metadata": []hostclient.Instance{{Name: "w-" + ws.ID, Status: "Running"}}, "partial": tc.partial})
			}))
			defer srv.Close()
			a.host = hostclient.New(srv.URL)
			_, err = a.runWorkspaceActionJob(ctx, j, func(string) {})
			if tc.want == "failed" && !jobs.IsPermanent(err) {
				t.Fatalf("expected terminal failure: %v", err)
			}
			if tc.want != "failed" && err != nil {
				t.Fatal(err)
			}
			got, _ := d.WorkspaceByID(ctx, ws.ID)
			if got.State != tc.want || posts != tc.posts {
				t.Fatalf("state=%s posts=%d", got.State, posts)
			}
			// Recovery from a committed completion does not repeat the provider call.
			saved, _ := a.jobs.Get(ctx, j.ID)
			saved.LeaseOwner = "test"
			a.runWorkspaceActionJob(ctx, saved, func(string) {})
			if posts != tc.posts {
				t.Fatal("completion replay dispatched again")
			}
		})
	}
}
