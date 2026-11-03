package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"sync/atomic"
	"testing"
)

func TestLifecycleLeaseLossAndProviderFailure(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		before, after, providerFail bool
	}{
		{"lease lost before dispatch", true, false, false}, {"lease lost after dispatch", false, true, false}, {"provider unavailable", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, d, tn := notifyAPI(t)
			a.jobs = jobs.NewStore(d.Pool)
			ws := provisioningWS(t, a, d, tn)
			if err := d.SetWorkspaceState(t.Context(), ws.ID, "stopped", "", ""); err != nil {
				t.Fatal(err)
			}
			j, err := a.jobs.EnqueueAction(t.Context(), jobs.Job{TenantID: tn.ID, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 3, Payload: json.RawMessage(`{"action":"start"}`)}, "starting")
			if err != nil {
				t.Fatal(err)
			}
			j.LeaseOwner = "worker"
			if _, err := d.Pool.Exec(t.Context(), `UPDATE jobs SET status='running',lease_owner='worker',lease_until=now()+interval '1 minute' WHERE id=$1`, j.ID); err != nil {
				t.Fatal(err)
			}
			expire := func() {
				if _, err := d.Pool.Exec(t.Context(), `UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, j.ID); err != nil {
					t.Error(err)
				}
			}
			if tc.before {
				expire()
			}
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts.Add(1)
					if tc.after {
						expire()
					}
					if tc.providerFail {
						w.WriteHeader(503)
					}
					w.Write([]byte(`{}`))
					return
				}
				t.Error("must not verify after failed dispatch/lease loss")
				w.WriteHeader(503)
			}))
			defer srv.Close()
			a.host = hostclient.New(srv.URL)
			_, err = a.runWorkspaceActionJob(t.Context(), j, func(string) {})
			if tc.providerFail {
				if !jobs.IsPermanent(err) {
					t.Fatalf("provider failure %v", err)
				}
			} else if !errors.Is(err, jobs.ErrLostLease) {
				t.Fatalf("lease failure %v", err)
			}
			wantPosts := int32(1)
			if tc.before {
				wantPosts = 0
			}
			if posts.Load() != wantPosts {
				t.Fatal("unexpected dispatch count", posts.Load())
			}
			row, _ := d.WorkspaceByID(t.Context(), ws.ID)
			want := "starting"
			if tc.providerFail {
				want = "failed"
			}
			if row.State != want {
				t.Fatalf("state=%s", row.State)
			}
		})
	}
}
