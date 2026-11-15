package proxmox

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The JSON below is shaped as Proxmox VE 9 answers: loadavg as strings, rrddata points that leave out
// a value they don't have, and a virtual machine's disk reported as 0.
func TestHostMetricsReadStatusAndTheNewestAveragedNetwork(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	rrd := `{"data":[{"time":1799999880,"netin":100.5,"netout":200.25},{"time":1799999940,"netin":110,"netout":210},{"time":1800000000,"cpu":0.1}]}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/n1/status":
			io.WriteString(w, `{"data":{"cpu":0.0625,"loadavg":["1.50","0.75","0.25"],"cpuinfo":{"cpus":16},
				"memory":{"total":64000,"used":16000,"free":48000},"rootfs":{"total":1000,"used":250,"avail":750}}}`)
		case "/api2/json/nodes/n1/rrddata":
			if r.URL.Query().Get("timeframe") != "hour" || r.URL.Query().Get("cf") != "AVERAGE" {
				t.Errorf("rrddata query %s", r.URL.RawQuery)
			}
			io.WriteString(w, rrd)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	c.Now = func() time.Time { return now }
	m, err := c.HostMetrics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Node != "n1" || m.CPUs != 16 || *m.CPU != 0.0625 || *m.Load1 != 1.5 || *m.MemoryUsed != 16000 || *m.MemoryTotal != 64000 ||
		*m.DiskUsed != 250 || *m.DiskTotal != 1000 || *m.NetInRate != 110 || *m.NetOutRate != 210 {
		t.Fatalf("%+v", m)
	}

	// A stale average is not reported as current traffic, and a token without rrddata keeps the rest.
	c.Now = func() time.Time { return now.Add(time.Hour) }
	if m, err = c.HostMetrics(context.Background()); err != nil || m.NetInRate != nil || m.CPU == nil {
		t.Fatalf("stale: %+v %v", m, err)
	}
	rrd = `{"data":[]}`
	if m, err = c.HostMetrics(context.Background()); err != nil || m.NetInRate != nil || m.NetOutRate != nil || m.Load1 == nil {
		t.Fatalf("no rrd: %+v %v", m, err)
	}
}

func TestHostMetricsLeaveOutWhatTheStatusDoesNotSay(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/n1/status":
			io.WriteString(w, `{"data":{"cpuinfo":{"cpus":4}}}`)
		default:
			http.Error(w, "permission denied", http.StatusForbidden)
		}
	}))
	defer srv.Close()
	m, err := clientFor(t, srv).HostMetrics(context.Background())
	if err != nil || m.CPU != nil || m.Load1 != nil || m.MemoryUsed != nil || m.DiskTotal != nil || m.NetInRate != nil {
		t.Fatalf("missing values must stay missing: %+v %v", m, err)
	}
}

func TestGuestReadingsFromClusterResources(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/resources" || r.URL.Query().Get("type") != "vm" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"data":[
			{"vmid":101,"type":"qemu","status":"running","node":"n2","cpu":0.5,"maxcpu":4,"mem":2048,"maxmem":4096,"disk":0,"maxdisk":32000,"netin":1000,"netout":500},
			{"vmid":102,"type":"lxc","status":"running","node":"n1","cpu":0.25,"maxcpu":2,"mem":512,"maxmem":1024,"disk":300,"maxdisk":8000,"netin":10,"netout":20},
			{"vmid":9000,"type":"qemu","status":"stopped","node":"n1","template":1,"maxdisk":1000}]}`)
	}))
	defer srv.Close()
	gs, err := clientFor(t, srv).GuestReadings(context.Background())
	if err != nil || len(gs) != 2 {
		t.Fatalf("%+v %v", gs, err)
	}
	vm, ct := gs[0], gs[1]
	if vm.ID != "101" || vm.Node != "n2" || vm.Status != "running" || *vm.CPU != 0.5 || vm.CPULimit != 4 || *vm.MemoryUsed != 2048 ||
		*vm.MemoryTotal != 4096 || vm.DiskUsed != nil || *vm.DiskTotal != 32000 || *vm.NetIn != 1000 || *vm.NetOut != 500 || vm.CPUSeconds != nil {
		t.Fatalf("vm: %+v", vm)
	}
	if ct.ID != "102" || *ct.DiskUsed != 300 || *ct.DiskTotal != 8000 {
		t.Fatalf("container: %+v", ct)
	}
}
