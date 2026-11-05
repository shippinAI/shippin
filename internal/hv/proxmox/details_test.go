package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetailsReadNodeAndCluster(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"node": "n2"}, {"node": "n1"}}})
		case "/api2/json/nodes/n1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"pveversion": "pve-manager/9.2.11/abc (running kernel: 7.0.14-14-pve)", "kversion": "Linux 7.0.14-14-pve #1 SMP", "uptime": 3600,
				"cpuinfo": map[string]any{"model": "Xeon", "cpus": 8}, "memory": map[string]any{"total": 100, "used": 40}}})
		case "/api2/json/nodes/n1/disks/list":
			if r.URL.Query().Get("skipsmart") != "1" {
				t.Errorf("overview disk inventory must skip SMART probes: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"devpath": "/dev/nvme0n1", "model": "Example NVMe", "size": 1000000, "used": "LVM", "mounted": true, "gpt": true},
				{"devpath": "", "size": 42},
			}})
		case "/api2/json/cluster/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"type": "cluster", "name": "lab", "nodes": 2, "quorate": true}, {"type": "node", "name": "n2", "online": 0}, {"type": "node", "name": "n1", "online": 1, "local": 1}}})
		case "/api2/json/nodes/n1/replication":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "101-0", "guest": 101, "target": "n2", "schedule": "*/15", "last_sync": 1700000000, "fail_count": 2, "error": "last run failed", "pid": 0},
				{"id": "102-0", "guest": 102, "pid": 45},
				{"id": "bad", "guest": 0},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	nodes, err := c.Nodes(context.Background())
	if err != nil || len(nodes) != 2 || nodes[0] != "n1" {
		t.Fatalf("%v %v", nodes, err)
	}
	c.Endpoint.Node = "n1"
	d, err := c.Details(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Release != "pve-manager/9.2.11/abc (running kernel: 7.0.14-14-pve)" || d.Kernel != "7.0.14-14-pve" || d.CPUs != 8 || d.MemTotal != 100 || d.Uptime != 3600 {
		t.Fatalf("%+v", d)
	}
	if d.Cluster != "lab" || len(d.Members) != 2 || d.Members[0].Name != "n1" || !d.Members[0].Online || !d.Members[0].Local || d.Members[1].Online {
		t.Fatalf("cluster: %+v", d)
	}
	if d.Quorate == nil || !*d.Quorate || d.ReplicationStatusAvailable == nil || !*d.ReplicationStatusAvailable || len(d.ReplicationJobs) != 2 || d.ReplicationJobs[0].ID != "101-0" || !d.ReplicationJobs[1].Running || d.ReplicationJobs[0].FailCount != 2 || d.ReplicationJobs[0].Error != "last run failed" {
		t.Fatalf("cluster observations: %+v", d)
	}
	if len(d.Disks) != 1 || d.Disks[0].Path != "/dev/nvme0n1" || d.Disks[0].Model != "Example NVMe" || d.Disks[0].Size != 1000000 || d.Disks[0].Used != "LVM" || !d.Disks[0].Mounted || !d.Disks[0].GPT {
		t.Fatalf("disk inventory: %+v", d.Disks)
	}
}

func TestDetailsKeepCoreFactsWhenOptionalClusterObservationsAreUnavailable(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/n1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"pveversion": "pve-manager/9"}})
		case "/api2/json/cluster/status", "/api2/json/nodes/n1/replication":
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	c.Endpoint.Node = "n1"
	d, err := c.Details(context.Background())
	if err != nil || d.Release != "pve-manager/9" || d.Quorate != nil || d.ReplicationStatusAvailable == nil || *d.ReplicationStatusAvailable || d.ReplicationJobs != nil {
		t.Fatalf("details=%+v err=%v", d, err)
	}
}

func TestDetailsBoundReplicationInventoryAndRejectMalformedOptionalResponses(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/n1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"pveversion": "pve-manager/9"}})
		case "/api2/json/cluster/status":
			_, _ = w.Write([]byte(`{"data":[{"type":"cluster","name":"lab","quorate":"unknown"}]}`))
		case "/api2/json/nodes/n1/replication":
			jobs := make([]map[string]any, maxReplicationJobs+1)
			for i := range jobs {
				jobs[i] = map[string]any{"id": fmt.Sprintf("%03d-0", i+1), "guest": i + 1}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": jobs})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	c.Endpoint.Node = "n1"
	d, err := c.Details(context.Background())
	if err != nil || d.Quorate != nil || d.ReplicationStatusAvailable == nil || !*d.ReplicationStatusAvailable || len(d.ReplicationJobs) != maxReplicationJobs {
		t.Fatalf("details=%+v err=%v", d, err)
	}
}

func TestDetailsSurviveATokenThatCannotReadTheCluster(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/n1/status" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"pveversion": "pve-manager/9"}})
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	c.Endpoint.Node = "n1"
	d, err := c.Details(context.Background())
	if err != nil || d.Release != "pve-manager/9" || d.Cluster != "" || d.Members != nil || d.Disks != nil {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestDetailsReportAStatusFailure(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) }))
	defer srv.Close()
	c := clientFor(t, srv)
	c.Endpoint.Node = "n1"
	if _, err := c.Details(context.Background()); err == nil {
		t.Fatal("a refused status read must be an error")
	}
}
