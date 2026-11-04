package proxmox

import (
	"encoding/json"
	"testing"
)

func TestGuestRetainsProxmoxInventoryTelemetry(t *testing.T) {
	var r resource
	if err := json.Unmarshal([]byte(`{"vmid":101,"name":"db","type":"qemu","status":"running","node":"pve-1","pool":"databases","tags":"prod;postgres","template":0,"uptime":3600,"cpu":0.25,"maxcpu":4,"mem":1024,"maxmem":4096,"disk":2048,"maxdisk":8192,"netin":20,"netout":30}`), &r); err != nil {
		t.Fatal(err)
	}
	g := r.guest()
	if g.ID != "101" || g.Pool != "databases" || g.Tags != "prod;postgres" || g.Template || g.Uptime != 3600 {
		t.Fatalf("resource identity: %+v", g)
	}
	if g.Metrics == nil || g.Metrics.CPU != .25 || g.Metrics.CPUCount != 4 || g.Metrics.Memory != 1024 || g.Metrics.MaxMemory != 4096 || g.Metrics.Disk != 2048 || g.Metrics.MaxDisk != 8192 || g.Metrics.NetworkIn != 20 || g.Metrics.NetworkOut != 30 {
		t.Fatalf("resource telemetry: %+v", g.Metrics)
	}
}
