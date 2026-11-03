package hostd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestQueryAllOmitsProject(t *testing.T) {
	at := place{t: tid, vm: true}
	if at.queryAll(context.Background(), "/1.0/instances?project=ws", nil) == nil {
		t.Fatal("project= accepted on an all-projects query")
	}
	f := useFake(t, fakeReply{match: "all-projects=true", out: `[]`})
	var list []innerInst
	if err := at.queryAll(context.Background(), "/1.0/instances?recursion=1", &list); err != nil {
		t.Fatal(err)
	}
	c := f.find("all-projects=true")
	j := strings.Join(c, " ")
	if c == nil || strings.Contains(j, "project=ws") || slices.Contains(c, "--project=ws") {
		t.Fatalf("query pinned a project: %v", c)
	}
}

func TestInventoryRejectsUnknown(t *testing.T) {
	ok, err := inventory([]innerInst{
		{Name: "w-" + wsID, Project: "pj-" + pjID},
		{Name: "u-" + pjID, Project: "pj-" + pjID},
	})
	if err != nil || len(ok) != 2 || ok[0].Kind != "workspace" || ok[1].Kind != "utility" {
		t.Fatalf("%+v %v", ok, err)
	}
	if _, err := inventory([]innerInst{{Name: "evil", Project: "ws"}}); err == nil {
		t.Fatal("unknown instance accepted")
	}
	if _, err := inventory([]innerInst{{Name: "w-not-a-uuid", Project: "ws"}}); err == nil {
		t.Fatal("non-uuid workspace accepted")
	}
	vm := place{t: tid, vm: true}
	if _, err := vm.atProject("default"); err == nil {
		t.Fatal("default project accepted")
	}
	got, err := vm.atProject("pj-" + pjID)
	if err != nil || got.project() != "pj-"+pjID {
		t.Fatalf("%s %v", got.project(), err)
	}
	if p, err := vm.atProject(InnerProject); err != nil || p.project() != "ws" {
		t.Fatal("legacy ws project")
	}
}

func TestWaitInnerListsAllProjects(t *testing.T) {
	list := `[{"name":"w-` + wsID + `","project":"pj-` + pjID + `","status":"Running","config":{"volatile.last_state.power":"RUNNING"}}]`
	f := useFake(t, fakeReply{match: "all-projects=true", out: list})
	if err := waitInner(context.Background(), tid); err != nil {
		t.Fatal(err)
	}
	c := f.find("all-projects=true")
	if c == nil || slices.Contains(c, "--project=ws") || strings.Contains(strings.Join(c, " "), "project=ws") {
		t.Fatalf("wait query: %v", c)
	}
}

func TestDeleteTenantVMAllProjects(t *testing.T) {
	s := testServer()
	s.cfg.StateDir = t.TempDir() // SetPorts persists the edge port table
	acct := pjID
	list := `[{"name":"w-` + wsID + `","project":"pj-` + pjID + `","status":"Running"},{"name":"u-` + acct + `","project":"pj-` + acct + `","status":"Stopped"}]`
	f := useFake(t,
		fakeReply{match: "/1.0/projects/" + Project(tid), out: `{"config":{"user.shippin.mode":"vm"}}`},
		fakeReply{match: "/1.0/networks/", out: `{"config":{"user.shippin.inner_subnet":"10.128.9.0/24","ipv4.address":"10.128.3.1/24"}}`},
		fakeReply{match: "all-projects=true", out: list},
		fakeReply{match: "/1.0/network-acls/", out: `{"ingress":[]}`},
		// remove() refuses to delete from an empty or unreadable inventory.
		fakeReply{match: "/servers/srv0/routes", out: `[]`},
		fakeReply{match: "/1.0/instances?recursion=1&project=pj-" + pjID, out: `[{"name":"w-` + wsID + `"}]`},
		fakeReply{match: "/volumes/custom?recursion=1&project=pj-" + pjID, out: `[]`},
	)
	if err := s.deleteTenantVM(context.Background(), tid); err != nil {
		t.Fatal(err)
	}
	if f.find("delete", "w-"+wsID, "--project=pj-"+pjID) == nil {
		t.Fatalf("workspace not deleted in its project: %v", f.calls)
	}
	if f.find("DELETE", "/id/u-"+acct) == nil {
		t.Fatalf("utility route kept: %v", f.calls)
	}
	if f.find("--project=ws", "delete", "w-"+wsID) != nil {
		t.Fatal("workspace delete was aimed at the legacy ws project")
	}
}

func TestDeleteTenantVMStopsOnUnknown(t *testing.T) {
	s := testServer()
	f := useFake(t,
		fakeReply{match: "/1.0/projects/" + Project(tid), out: `{"config":{"user.shippin.mode":"vm"}}`},
		fakeReply{match: "/1.0/networks/", out: `{"config":{"user.shippin.inner_subnet":"10.128.9.0/24","ipv4.address":"10.128.3.1/24"}}`},
		fakeReply{match: "all-projects=true", out: `[{"name":"evil","project":"ws","status":"Running"}]`},
	)
	if err := s.deleteTenantVM(context.Background(), tid); err == nil {
		t.Fatal("unknown instance did not stop the delete")
	}
	if f.find("delete", TenantVM(tid)) != nil {
		t.Fatal("VM deleted while an unknown instance was still listed")
	}
}

func TestTenantInstancesUsesQueryAll(t *testing.T) {
	s := testServer()
	f := useFake(t,
		fakeReply{match: "/1.0/projects/" + Project(tid), out: `{"config":{"user.shippin.mode":"vm"}}`},
		fakeReply{match: "/1.0/networks/", out: `{"config":{"user.shippin.inner_subnet":"10.128.9.0/24","ipv4.address":"10.128.3.1/24"}}`},
		fakeReply{match: "/1.0/instances?project=", out: `[{"name":"` + TenantVM(tid) + `","status":"Running"}]`},
		fakeReply{match: "all-projects=true", out: `[{"name":"w-` + wsID + `","project":"pj-` + pjID + `"}]`},
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tenants/"+tid+"/instances", nil)
	req.SetPathValue("t", tid)
	s.tenantInstances(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "w-"+wsID) || strings.Contains(body, `"partial":true`) {
		t.Fatalf("body %s calls %v", body, f.calls)
	}
	c := f.find("all-projects=true")
	if c == nil || strings.Contains(strings.Join(c, " "), "all-projects=true&project=") || strings.Contains(strings.Join(c, " "), "project=ws") {
		t.Fatalf("instances query still pins a project: %v", c)
	}
}

func TestStartDoesNotForceAutostart(t *testing.T) {
	s := testServer()
	f := useFake(t)
	if err := s.Action(context.Background(), tid, wsid, "start", WorkspaceReq{}); err != nil {
		t.Fatal(err)
	}
	if f.find("boot.autostart=true") != nil {
		t.Fatal("start must not force boot.autostart=true (that would start stopped devspaces)")
	}
	if f.find("shippin-sessions") != nil {
		t.Fatal("session manager booted with the flag off")
	}
}

func TestCapSize(t *testing.T) {
	p := Plan{WorkspaceCPU: 4, WorkspaceMem: "8GiB", QuotaGiB: 200, VM: &VMPlan{DataGiB: 150}}
	got := capSize(p, Size{CPU: 64, MemoryGiB: 256, DiskGiB: 2048})
	if got != (Size{CPU: 4, MemoryGiB: 8, DiskGiB: 150}) {
		t.Fatalf("cap %v", got)
	}
	if z := capSize(p, Size{}); z != (Size{}) {
		t.Fatalf("zero size is the profile default: %v", z)
	}
	if z := capSize(p, Size{CPU: 2, MemoryGiB: 4, DiskGiB: 20}); z.CPU != 2 || z.MemoryGiB != 4 || z.DiskGiB != 20 {
		t.Fatalf("under the cap changed: %v", z)
	}
}
