package api

import (
	"net/http"
	"net/http/httptest"
	"shippin.cloud/console/internal/hostclient"
	"strings"
	"testing"
)

func TestWorkspaceInventoryIncomplete(t *testing.T) {
	for _, tc := range []struct {
		name, body  string
		code        int
		observation string
	}{
		{"partial", `{"metadata":[{"name":"unused","status":"Running"}],"partial":true}`, 200, "unavailable"},
		{"offline", `{}`, 503, "unavailable"}, {"missing", `{"metadata":[],"partial":false}`, 200, "missing"},
		{"malformed", `broken`, 200, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, d, tn := notifyAPI(t)
			ws := provisioningWS(t, a, d, tn)
			if err := d.SetWorkspaceState(t.Context(), ws.ID, "running", "", ""); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				w.Write([]byte(strings.ReplaceAll(tc.body, "unused", "w-"+ws.ID)))
			}))
			defer srv.Close()
			a.host = hostclient.New(srv.URL)
			rows, err := a.liveViews(httptest.NewRequest("GET", "/", nil), &tenantView{Tenant: tn})
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%d err=%v", len(rows), err)
			}
			if rows[0].State != "unknown" || rows[0].Observation != tc.observation || rows[0].RecordedState != "running" || rows[0].ObservedAt != nil {
				t.Fatalf("misleading inventory: %+v", rows[0])
			}
		})
	}
}
