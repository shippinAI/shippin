package hostd

import (
	"net/http/httptest"
	"strings"
	"testing"
)

const hostMetrics = `# HELP incus_cpu_seconds_total x
incus_cpu_seconds_total{cpu="0",mode="user",name="tvm-a",project="t-a",type="virtual-machine"} 3600
incus_cpu_seconds_total{cpu="0",mode="idle",name="tvm-a",project="t-a",type="virtual-machine"} 99999
incus_memory_MemTotal_bytes{name="tvm-a",project="t-a",type="virtual-machine"} 4294967296
incus_memory_MemAvailable_bytes{name="tvm-a",project="t-a",type="virtual-machine"} 2147483648
`

const innerMetrics = `incus_cpu_seconds_total{cpu="0",mode="system",name="w-1",project="pj-p1",type="container"} 1800
incus_memory_RSS_bytes{name="w-1",project="pj-p1",type="container"} 1073741824
incus_filesystem_size_bytes{device="r",fstype="zfs",mountpoint="/",name="w-1",project="pj-p1",type="container"} 21474836480
incus_filesystem_avail_bytes{device="r",fstype="zfs",mountpoint="/",name="w-1",project="pj-p1",type="container"} 10737418240
incus_filesystem_size_bytes{device="h",fstype="zfs",mountpoint="/tmp",name="w-1",project="pj-p1",type="container"} 999
incus_network_transmit_bytes_total{device="eth0",name="w-1",project="pj-p1",type="container"} 1073741824
incus_network_transmit_bytes_total{device="lo",name="w-1",project="pj-p1",type="container"} 555555
incus_procs_total{name="s-1",project="pj-p1",type="container"} 7
incus_cpu_seconds_total{cpu="0",mode="user",name="x",project="default",type="container"} 5
garbage line
`

func TestParseMetricsFoldsPerInstance(t *testing.T) {
	us := ParseMetrics(hostMetrics + innerMetrics)
	if len(us) != 4 {
		t.Fatalf("%d instances: %+v", len(us), us)
	}
	vm, w := us[0], us[1]
	if vm.CPUSec != 3600 || vm.MemBytes != 2*gib {
		t.Fatalf("vm %+v", vm)
	}
	if w.CPUSec != 1800 || w.MemBytes != gib || w.DiskBytes != 10*gib || w.TxBytes != gib {
		t.Fatalf("workspace %+v", w)
	}
}

func TestRollupPricesPerProject(t *testing.T) {
	ps := Rollup(ParseMetrics(innerMetrics), CostRates{CPUHour: 1})
	var p1 *ProjectUsage
	for i := range ps {
		if ps[i].ProjectID == "p1" {
			p1 = &ps[i]
		}
	}
	if p1 == nil || len(p1.Instances) != 2 {
		t.Fatalf("%+v", ps)
	}
	// 0.5 CPU-h at $1 + 1 GiB egress at default $0.01; 1 GiB mem * 2.5 + 10 GiB disk * 0.08.
	if p1.AccruedUSD != 0.51 || p1.MonthlyUSD != 3.3 {
		t.Fatalf("cost %v %v", p1.AccruedUSD, p1.MonthlyUSD)
	}
}

func TestTenantMetricsVMTenant(t *testing.T) {
	f := useFake(t,
		fakeReply{match: "query /1.0/metrics?project=t-", out: hostMetrics},
		fakeReply{match: "/1.0/projects/t-", out: `{"config":{"user.shippin.mode":"vm"}}`},
		fakeReply{match: "/1.0/networks/", out: `{"config":{"user.shippin.inner_subnet":"10.128.9.0/24"}}`},
		fakeReply{match: "incus query /1.0/metrics", out: innerMetrics},
	)
	rr := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/tenants/"+tid+"/metrics", nil))
	b := rr.Body.String()
	if rr.Code != 200 || !strings.Contains(b, `"project_id":"p1"`) || !strings.Contains(b, `"name":"tvm-a"`) {
		t.Fatalf("%d %s", rr.Code, b)
	}
	if strings.Contains(b, `"project":"default"`) {
		t.Fatal("inner default project leaked into tenant metrics")
	}
	if f.find("exec", "tvm-"+tid, "query", "/1.0/metrics") == nil {
		t.Fatal("inner metrics not read through the VM agent")
	}
}

// B2 precheck: a devspace's home volume is a second filesystem series at /home; it is billed disk.
func TestParseMetricsCountsHomeVolume(t *testing.T) {
	us := ParseMetrics(`incus_filesystem_size_bytes{device="r",fstype="zfs",mountpoint="/",name="w-9",project="t-a",type="container"} 3
incus_filesystem_avail_bytes{device="r",fstype="zfs",mountpoint="/",name="w-9",project="t-a",type="container"} 1
incus_filesystem_size_bytes{device="h",fstype="zfs",mountpoint="/home",name="w-9",project="t-a",type="container"} 10
incus_filesystem_avail_bytes{device="h",fstype="zfs",mountpoint="/home",name="w-9",project="t-a",type="container"} 4
incus_filesystem_size_bytes{device="x",fstype="zfs",mountpoint="/srv",name="w-9",project="t-a",type="container"} 100
`)
	if len(us) != 1 || us[0].DiskBytes != 8 {
		t.Fatalf("%+v", us)
	}
}
