package devspace

import (
	"strings"
	"testing"

	"shippin.cloud/console/internal/ports"
)

// A running or stopped space is shown in the state its host reports, and as unknown when the host
// could not be read or reports something else. The stored state is never shown as if it were live.
func TestShowObservation(t *testing.T) {
	a, tn := Reader{}, Org{}
	for _, state := range []string{"running", "stopped"} {
		ws := Devspace{ID: "w", State: state}
		for _, live := range []map[string]ports.Guest{nil, {}} {
			got := a.Show(tn, ws, live)
			if got.State != "unknown" || got.RecordedState != state || got.ObservedAt != nil {
				t.Fatalf("stale state: %+v", got)
			}
		}
		got := a.Show(tn, ws, map[string]ports.Guest{"w-w": {Status: "Stopped"}})
		if got.State != "stopped" || got.Observation != "observed" || got.ObservedAt == nil {
			t.Fatalf("live state: %+v", got)
		}
	}
	for _, status := range []string{"Frozen", "", "Error"} {
		got := a.Show(tn, Devspace{ID: "w", State: "running"}, map[string]ports.Guest{"w-w": {Status: status}})
		if got.State != "unknown" || got.Observation != "observed" {
			t.Fatalf("unsupported provider state: %+v", got)
		}
	}
	got := a.Show(tn, Devspace{State: "starting"}, nil)
	if got.State != "starting" || got.Observation != "unavailable" {
		t.Fatalf("lost operation: %+v", got)
	}
}

// The steps are the server's: one running step while a space is being made, the failed one with its
// reason, and none for a space with no work in progress.
func TestShowSteps(t *testing.T) {
	r := Reader{}
	s := r.Show(Org{}, Devspace{State: "provisioning", Repo: "https://example.test/a/b", Progress: "Waiting for network (3s)"}, nil).Steps
	var states []string
	for _, st := range s {
		states = append(states, st.State)
	}
	if got := strings.Join(states, " "); got != "done done done running pending pending pending" {
		t.Fatalf("provisioning with a repository: %s", got)
	}
	if s[3].Label != "Waiting for network (3s)" || s[5].Label != "Cloning" || s[6].Label != "Publishing HTTPS routes" {
		t.Fatalf("labels: %+v", s)
	}
	if s := r.Show(Org{}, Devspace{State: "provisioning"}, nil).Steps; len(s) != 6 || s[0].State != "running" || s[1].State != "pending" {
		t.Fatalf("provisioning before the first step: %+v", s)
	}
	if s := r.Show(Org{}, Devspace{State: "failed", Error: "no room"}, nil).Steps; len(s) != 1 || s[0].State != "failed" || s[0].Detail != "no room" {
		t.Fatalf("failed: %+v", s)
	}
	if s := r.Show(Org{}, Devspace{State: "running"}, nil).Steps; s == nil || len(s) != 0 {
		t.Fatalf("running: %+v", s)
	}
}
