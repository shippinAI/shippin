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

func TestLifecycleUsesExactRecordedGrant(t *testing.T) {
	for _, mode := range []string{"valid with preferred extra grant", "wrong requester", "expired", "wrong tenant"} {
		t.Run(mode, func(t *testing.T) {
			a, d, tn := notifyAPI(t)
			a.jobs = jobs.NewStore(d.Pool)
			ws := provisioningWS(t, a, d, tn)
			if e := d.SetWorkspaceState(t.Context(), ws.ID, "stopped", "", ""); e != nil {
				t.Fatal(e)
			}
			g := db.Grant{ID: newUUID(), TenantID: tn.ID, WorkspaceID: ws.ID, Kind: "request", Requester: "u", RequesterName: "test", Reason: "fixture", DurationS: 3600, Status: "approved"}
			if mode == "wrong requester" {
				g.Requester = "other"
			}
			if mode == "wrong tenant" {
				other := db.Tenant{ID: newUUID(), IdpOrgID: newUUID(), Name: "Other", Plan: "p"}
				if e := d.CreateTenant(t.Context(), other); e != nil {
					t.Fatal(e)
				}
				g.TenantID = other.ID
				g.WorkspaceID = ""
			}
			if e := d.CreateGrant(t.Context(), g); e != nil {
				t.Fatal(e)
			}
			if mode == "expired" {
				if _, e := d.Pool.Exec(t.Context(), `UPDATE access_grants SET expires_at=now()-interval '1 second' WHERE id=$1`, g.ID); e != nil {
					t.Fatal(e)
				}
			}
			extra := g
			extra.ID = newUUID()
			extra.WorkspaceID = ""
			extra.Kind = "breakglass"
			extra.Requester = "u"
			if e := d.CreateGrant(t.Context(), extra); e != nil {
				t.Fatal(e)
			}
			payload, _ := json.Marshal(workspaceActionIn{Action: "start", Grant: g.ID})
			j, e := a.jobs.EnqueueAction(t.Context(), jobs.Job{TenantID: tn.ID, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 3, Payload: payload}, "starting")
			if e != nil {
				t.Fatal(e)
			}
			j.LeaseOwner = "worker"
			if _, e := d.Pool.Exec(t.Context(), `UPDATE jobs SET status='running',lease_owner='worker',lease_until=now()+interval '1 minute' WHERE id=$1`, j.ID); e != nil {
				t.Fatal(e)
			}
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					w.Write([]byte(`{}`))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"metadata": []hostclient.Instance{{Name: "w-" + ws.ID, Status: "Running"}}, "partial": false})
			}))
			defer srv.Close()
			a.host = hostclient.New(srv.URL)
			_, e = a.runWorkspaceActionJob(t.Context(), j, func(string) {})
			if mode == "valid with preferred extra grant" {
				if e != nil || posts != 1 {
					t.Fatalf("valid recorded grant: posts=%d err=%v", posts, e)
				}
			} else if !jobs.IsPermanent(e) || posts != 0 {
				t.Fatalf("invalid grant dispatched: posts=%d err=%v", posts, e)
			}
		})
	}
}
