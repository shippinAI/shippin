package api

import (
	"testing"

	"shippin.cloud/console/internal/hostclient"
)

func TestNestedStates(t *testing.T) {
	before := &hostclient.TenantVM{Status: "Running", Containers: []hostclient.VMContainer{
		{Workspace: "a", Status: "Running"}, {Workspace: "b", Status: "Stopped"}, {Workspace: "", Status: "Running"}}}
	stopped := &hostclient.TenantVM{Status: "Stopped"}
	got := nestedStates(before, stopped)
	if len(got) != 2 || got["a"] != "stopped" || got["b"] != "stopped" {
		t.Fatalf("after stop %v", got)
	}
	started := &hostclient.TenantVM{Status: "Running", Containers: []hostclient.VMContainer{{Workspace: "a", Status: "Running"}}}
	got = nestedStates(nil, started)
	if got["a"] != "running" || len(got) != 1 {
		t.Fatalf("after start %v", got)
	}
	if len(nestedStates(nil, nil)) != 0 {
		t.Fatal("nil views")
	}
}
