package incus

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The instance JSON is shaped as Incus 6 answers /1.0/instances?recursion=2: each instance carries
// its state, with CPU time in nanoseconds and per-interface counters.
func TestGuestReadingsComeFromOneCallWithState(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1.0/instances" {
			t.Errorf("unexpected call %s", r.URL)
			http.NotFound(w, r)
			return
		}
		calls++
		if r.URL.Query().Get("recursion") != "2" || r.URL.Query().Get("project") != "tenants" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		io.WriteString(w, `{"type":"sync","metadata":[
			{"name":"web","status":"Running","type":"container","location":"none","expanded_config":{"limits.cpu":"0-1,4"},
			 "state":{"status":"Running","cpu":{"usage":2500000000},"memory":{"usage":1048576,"total":4194304},
			  "disk":{"root":{"usage":5000,"total":20000}},
			  "network":{"lo":{"type":"loopback","counters":{"bytes_received":999,"bytes_sent":999}},
			   "eth0":{"type":"broadcast","counters":{"bytes_received":100,"bytes_sent":40}},
			   "eth1":{"type":"broadcast","counters":{"bytes_received":1,"bytes_sent":2}}}}},
			{"name":"vm1","status":"Running","type":"virtual-machine","location":"node2","expanded_config":{},
			 "state":{"status":"Running","cpu":{"usage":-1},"memory":{"usage":0},"disk":{"root":{"usage":10}},"network":{}}},
			{"name":"off","status":"Stopped","type":"container","expanded_config":{"limits.cpu":"3"},"state":{"status":"Stopped"}}]}`)
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	c.Endpoint.Project = "tenants"
	gs, err := c.GuestReadings(context.Background())
	if err != nil || len(gs) != 3 || calls != 1 {
		t.Fatalf("%+v %v (%d calls)", gs, err, calls)
	}
	web, vm, off := gs[0], gs[1], gs[2]
	if web.ID != "web" || web.Node != "" || web.Status != "Running" || web.CPU != nil || *web.CPUSeconds != 2.5 || web.CPULimit != 3 ||
		*web.MemoryUsed != 1048576 || *web.MemoryTotal != 4194304 || *web.DiskUsed != 5000 || *web.DiskTotal != 20000 || *web.NetIn != 101 || *web.NetOut != 42 {
		t.Fatalf("web: %+v", web)
	}
	// What Incus could not read for a virtual machine without its agent stays missing.
	if vm.Node != "node2" || vm.CPUSeconds != nil || vm.MemoryUsed != nil || *vm.DiskUsed != 10 || vm.DiskTotal != nil || vm.NetIn != nil || vm.CPULimit != 1 {
		t.Fatalf("vm: %+v", vm)
	}
	if off.Status != "Stopped" || off.CPULimit != 3 || off.CPUSeconds != nil {
		t.Fatalf("stopped: %+v", off)
	}
}

func TestHostMetricsReportOnlyWhatIncusExposes(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1.0":
			io.WriteString(w, `{"metadata":{"environment":{"server_name":"node1"}}}`)
		case "/1.0/resources":
			io.WriteString(w, `{"metadata":{"cpu":{"total":12},"memory":{"total":8000,"used":3000}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	m, err := clientFor(t, srv).HostMetrics(context.Background())
	if err != nil || m.Node != "node1" || m.CPUs != 12 || *m.MemoryUsed != 3000 || *m.MemoryTotal != 8000 {
		t.Fatalf("%+v %v", m, err)
	}
	if m.CPU != nil || m.Load1 != nil || m.DiskUsed != nil || m.NetInRate != nil {
		t.Fatalf("Incus does not report these, so they must stay missing: %+v", m)
	}
}

func TestCPULimit(t *testing.T) {
	for v, want := range map[string]int{"": 0, "4": 4, "0-3": 4, "0,2,4": 3, "0-1,6": 3, "0": 1, "x": 0, "3-1": 0} {
		if got := cpuLimit(v, false); got != want {
			t.Errorf("cpuLimit(%q) = %d, want %d", v, got, want)
		}
	}
	if cpuLimit("", true) != 1 {
		t.Error("a virtual machine without limits.cpu has one CPU")
	}
}

// A storage pool is listed with how full it is, and a pool that does not say is still listed.
func TestStorageReportsPoolSpace(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1.0/storage-pools":
			io.WriteString(w, `{"metadata":[{"name":"fast","driver":"zfs"},{"name":"odd","driver":"dir"}]}`)
		case "/1.0/storage-pools/fast/resources":
			io.WriteString(w, `{"metadata":{"space":{"used":3000,"total":8000}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	v, err := clientFor(t, srv).Storage(context.Background())
	if err != nil || len(v) != 2 || v[0].Name != "fast" || v[0].Used != 3000 || v[0].Total != 8000 || v[1].Name != "odd" || v[1].Total != 0 {
		t.Fatalf("%+v %v", v, err)
	}
}

// Networks are listed with their address and how many guests use them; loopback is left out.
func TestNetworksReportAddressAndUse(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"metadata":[
			{"name":"incusbr0","type":"bridge","managed":true,"used_by":["/1.0/instances/a","/1.0/profiles/default"],"config":{"ipv4.address":"10.0.0.1/24"}},
			{"name":"eno1","type":"physical","managed":false,"used_by":[],"config":{}},
			{"name":"lo","type":"loopback","managed":false}]}`)
	}))
	defer srv.Close()
	n, err := clientFor(t, srv).Networks(context.Background())
	if err != nil || len(n) != 2 || n[0].Name != "incusbr0" || n[0].Address != "10.0.0.1/24" || !n[0].Managed || n[0].Guests != 2 || n[1].Name != "eno1" || n[1].Managed {
		t.Fatalf("%+v %v", n, err)
	}
}
