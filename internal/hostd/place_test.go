package hostd

import (
	"net/netip"
	"slices"
	"testing"
)

const tid = "019a0000-0000-7000-8000-00000000f001"

func TestPlaceArgv(t *testing.T) {
	host := place{t: tid}
	if got := host.argv([]string{"start", "w-x"}); !slices.Equal(got, []string{"--project=t-" + tid, "start", "w-x"}) {
		t.Fatalf("host argv %v", got)
	}
	vm := place{t: tid, vm: true}
	got := vm.argv([]string{"exec", "w-x", "--", "ss"})
	want := []string{"exec", "tvm-" + tid, "--project=t-" + tid, "--", "incus", "--project=ws", "exec", "w-x", "--", "ss"}
	if !slices.Equal(got, want) {
		t.Fatalf("vm argv %v", got)
	}
	q := vm.argv([]string{"query", "/1.0/instances?project=ws"})
	if !slices.Equal(q, []string{"exec", "tvm-" + tid, "--project=t-" + tid, "--", "incus", "query", "/1.0/instances?project=ws"}) {
		t.Fatalf("query argv %v", q)
	}
	if vm.pool() != "default" || host.pool() != "p-"+tid || vm.project() != "ws" {
		t.Fatal("pool/project mapping")
	}
}

func TestPlaceCheckIP(t *testing.T) {
	vm := place{t: tid, vm: true, inner: netip.MustParsePrefix("10.128.9.0/24")}
	for ip, ok := range map[string]bool{"10.128.9.7": true, "10.128.1.2": false, "198.51.100.10": false, "x": false, "": false} {
		if (vm.checkIP(ip) == nil) != ok {
			t.Errorf("%q ok=%v", ip, !ok)
		}
	}
	if (place{t: tid}).checkIP("10.128.1.2") != nil {
		t.Error("legacy placement must not restrict")
	}
}

func TestVMAddressing(t *testing.T) {
	if a, err := vmAddr("10.128.3.1/24"); err != nil || a != "10.128.3.254" {
		t.Fatalf("vmAddr %q %v", a, err)
	}
	if _, err := vmAddr("10.128.0.1/16"); err == nil {
		t.Fatal("non-/24 accepted")
	}
	if g, err := innerGateway("10.128.9.0/24"); err != nil || g != "10.128.9.1/24" {
		t.Fatalf("gateway %q %v", g, err)
	}
	if _, err := innerGateway("fd00::/64"); err == nil {
		t.Fatal("v6 accepted")
	}
}

func TestImageFilesMetaFirst(t *testing.T) {
	d := t.TempDir() + "/abc"
	mustWrite(t, d+"/abc.root")
	mustWrite(t, d+"/abc.tar.xz")
	f := imageFiles(d)
	if len(f) != 2 || f[0] != d+"/abc.tar.xz" {
		t.Fatalf("order %v", f)
	}
}

func TestConfigCheckVM(t *testing.T) {
	c := *testServer().cfg
	c.VMFirst = true
	if c.Check() == nil {
		t.Fatal("vm_first without image accepted")
	}
	c.TenantVMImage = "shippin/tenant-vm"
	c.Plans = map[string]Plan{"p": {QuotaGiB: 80, VM: &VMPlan{CPU: 2, Memory: "4GiB", RootGiB: 4, DataGiB: 20}}}
	if c.Check() == nil {
		t.Fatal("tiny root accepted")
	}
	c.Plans["p"].VM.RootGiB = 20
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestAutostarted(t *testing.T) {
	run := map[string]string{"volatile.last_state.power": "RUNNING"}
	if autostarted([]innerInst{{Name: "w-" + wsID, Status: "Stopped", Config: run}}) {
		t.Fatal("pending autostart reported done")
	}
	if !autostarted([]innerInst{
		{Name: "w-" + wsID, Status: "Running", Config: run},
		{Name: "w-" + pjID, Status: "Stopped", Config: map[string]string{"volatile.last_state.power": "STOPPED"}},
	}) {
		t.Fatal("stopped-by-user workspace must not block")
	}
	if !autostarted([]innerInst{{Name: "u-" + pjID, Status: "Stopped", Config: run}}) {
		t.Fatal("activation containers must not block VM ready")
	}
}

func TestPlacePerProject(t *testing.T) {
	pj := "019a0000-0000-7000-8000-0000000000aa"
	vm := place{t: tid, vm: true}.in(pj)
	if vm.project() != "pj-"+pj {
		t.Fatalf("project %q", vm.project())
	}
	got := vm.argv([]string{"start", "w-x"})
	want := []string{"exec", "tvm-" + tid, "--project=t-" + tid, "--", "incus", "--project=pj-" + pj, "start", "w-x"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv %v", got)
	}
	if (place{t: tid}).in(pj).project() != "t-"+tid {
		t.Fatal("legacy direct placement ignores the project")
	}
	if BoundaryVM(pj) != "b-"+pj || ProjectInner(pj) != "pj-"+pj {
		t.Fatal("names")
	}
}
