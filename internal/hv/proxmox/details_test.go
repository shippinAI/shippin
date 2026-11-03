package proxmox

import (
	"context"
	"encoding/json"
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
		case "/api2/json/cluster/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"type": "cluster", "name": "lab", "nodes": 2}, {"type": "node", "name": "n2", "online": 0}, {"type": "node", "name": "n1", "online": 1, "local": 1}}})
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
	if err != nil || d.Release != "pve-manager/9" || d.Cluster != "" || d.Members != nil {
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
