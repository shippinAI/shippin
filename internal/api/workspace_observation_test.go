package api

import (
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"testing"
)

func TestWorkspaceObservation(t *testing.T) {
	a := &API{cfg: testCfg()}
	tn := &tenantView{}
	for _, state := range []string{"running", "stopped"} {
		ws := db.Workspace{ID: "w", State: state}
		for _, live := range []map[string]hostclient.Instance{nil, {}} {
			got := a.view(tn, ws, live)
			if got.State != "unknown" || got.RecordedState != state || got.ObservedAt != nil {
				t.Fatalf("stale state: %+v", got)
			}
		}
		got := a.view(tn, ws, map[string]hostclient.Instance{"w-w": {Status: "Stopped"}})
		if got.State != "stopped" || got.Observation != "observed" || got.ObservedAt == nil {
			t.Fatalf("live state: %+v", got)
		}
	}
	for _, status := range []string{"Frozen", "", "Error"} {
		got := a.view(tn, db.Workspace{ID: "w", State: "running"}, map[string]hostclient.Instance{"w-w": {Status: status}})
		if got.State != "unknown" || got.Observation != "observed" {
			t.Fatalf("unsupported provider state: %+v", got)
		}
	}
	got := a.view(tn, db.Workspace{State: "starting"}, nil)
	if got.State != "starting" || got.Observation != "unavailable" {
		t.Fatalf("lost operation: %+v", got)
	}
}
