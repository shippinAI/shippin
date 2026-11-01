package hostd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func storageServer(t *testing.T) *Server {
	t.Helper()
	s := testServer()
	s.cfg.DatasetRoot = "tank/tenants"
	s.cfg.TenantVMInit = "/usr/local/share/shippin/shippin-tvm-init"
	s.cfg.Plans["pro"] = Plan{
		QuotaGiB: 160, Instances: 7, CPU: 8, Memory: "16GiB", RootGiB: 20, HomeGiB: 40,
		WorkspaceCPU: 4, WorkspaceMem: "8GiB", Cowork: true,
		VM: &VMPlan{CPU: 4, Memory: "16GiB", RootGiB: 20, DataGiB: 100},
	}
	return s
}

func TestGrowStorageUsesOrgPool(t *testing.T) {
	s := storageServer(t)
	f := useFake(t,
		fakeReply{match: "/1.0/projects/", out: `{"config":{"user.shippin.mode":"vm","user.shippin.plan":"pro"}}`},
		fakeReply{match: "/1.0/networks/", out: `{"config":{"user.shippin.inner_subnet":"10.200.1.0/24"}}`},
		fakeReply{match: "value quota", out: "42949672960\n"},
		fakeReply{match: "value used", out: "1073741824\n"},
		fakeReply{match: "volumes/custom/", out: `{"config":{"size":"40GiB"}}`},
		fakeReply{match: "/1.0/instances/", out: `{"expanded_devices":{"root":{"size":"20GiB"}},"config":{"limits.cpu":"4","limits.memory":"16GiB"}}`},
	)
	rep, err := s.GrowStorage(context.Background(), tid, StorageTarget{QuotaGiB: 100, RootGiB: 30, DataGiB: 60})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pool != "p-"+tid || rep.Dataset != "tank/tenants/"+tid || rep.CoworkDisk != "org-data-volume" {
		t.Fatalf("placement: %+v", rep)
	}
	if rep.LowSpace {
		t.Fatalf("not low: %+v", rep)
	}
	want := [][]string{
		{"zfs", "set", "quota=100G", "tank/tenants/" + tid},
		{"project", "set", "t-" + tid, "limits.disk.pool.p-" + tid + "=100GiB"},
		{"config", "device", "set", "tvm-" + tid, "root", "size=30GiB"},
		{"storage", "volume", "set", "p-" + tid, "tvd-" + tid, "size=60GiB"},
		{"shippin-tvm-init", "grow", tid},
	}
	for _, w := range want {
		if f.find(w...) == nil {
			t.Errorf("missing %v", w)
		}
	}
	for _, c := range f.calls {
		j := strings.Join(c, " ")
		if strings.Contains(j, "platform") {
			t.Fatalf("platform pool: %s", j)
		}
		if strings.Contains(j, "storage volume") && !strings.Contains(j, "p-"+tid) {
			t.Fatalf("volume not on the org pool: %s", j)
		}
	}
}

func TestGrowStorageRefusesShrink(t *testing.T) {
	s := storageServer(t)
	f := useFake(t,
		fakeReply{match: "/1.0/projects/", out: `{"config":{"user.shippin.mode":"vm","user.shippin.plan":"pro"}}`},
		fakeReply{match: "/1.0/networks/", out: `{"config":{"user.shippin.inner_subnet":"10.200.1.0/24"}}`},
		fakeReply{match: "value quota", out: "85899345920\n"},
		fakeReply{match: "value used", out: "1073741824\n"},
		fakeReply{match: "volumes/custom/", out: `{"config":{"size":"40GiB"}}`},
		fakeReply{match: "/1.0/instances/", out: `{"expanded_devices":{"root":{"size":"20GiB"}}}`},
	)
	_, err := s.GrowStorage(context.Background(), tid, StorageTarget{QuotaGiB: 40})
	if err == nil || !strings.Contains(err.Error(), "shrinking") {
		t.Fatal(err)
	}
	if f.find("zfs", "set") != nil {
		t.Fatal("shrink issued zfs set")
	}
}

func TestLowSpaceSignals(t *testing.T) {
	ok, why := lowSpace(100<<30, 95<<30, 0, 0, false)
	if !ok || !strings.Contains(why, "10%") {
		t.Fatalf("dataset: %v %s", ok, why)
	}
	ok, why = lowSpace(100<<30, 1<<30, 100<<30, 80<<30, true)
	if !ok || !strings.Contains(why, "25 GiB") {
		t.Fatalf("cowork: %v %s", ok, why)
	}
	if ok, _ := lowSpace(100<<30, 1<<30, 100<<30, 80<<30, false); ok {
		t.Fatal("cowork reservation applied to a plan without cowork")
	}
}

func TestOrgScratchStaysOnDataset(t *testing.T) {
	mp := filepath.Join(t.TempDir(), tid)
	useFake(t, fakeReply{match: "mountpoint", out: mp + "\n"})
	dir, err := datasetScratch(context.Background(), "tank/tenants", tid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, filepath.Join(tid, "scratch")) {
		t.Fatal(dir)
	}
	useFake(t, fakeReply{match: "mountpoint", out: "/tmp\n"})
	if _, err := datasetScratch(context.Background(), "tank/tenants", tid); err == nil {
		t.Fatal("foreign mountpoint accepted")
	}
}

func TestInnerImageStageIsDataVolume(t *testing.T) {
	if !strings.Contains(innerImageStage, "/storage-pools/"+InnerPool+"/") || strings.Contains(innerImageStage, "/root/") {
		t.Fatal(innerImageStage)
	}
}

func TestStorageAddRejectsShrinkOverHTTP(t *testing.T) {
	s := storageServer(t)
	useFake(t,
		fakeReply{match: "/1.0/projects/", out: `{"config":{"user.shippin.mode":"vm"}}`},
		fakeReply{match: "/1.0/networks/", out: `{"config":{"user.shippin.inner_subnet":"10.200.1.0/24"}}`},
		fakeReply{match: "value quota", out: "85899345920\n"},
		fakeReply{match: "value used", out: "1\n"},
		fakeReply{match: "volumes/custom/", out: `{"config":{"size":"40GiB"}}`},
		fakeReply{match: "/1.0/instances/", out: `{"expanded_devices":{"root":{"size":"20GiB"}}}`},
	)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants/"+tid+"/storage", strings.NewReader(`{"quota_gib":10}`))
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "shrinking") {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
}

func TestApplyPlanUnknown(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants/"+tid+"/plan", strings.NewReader(`{"plan":"micro"}`))
	testServer().Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
}
