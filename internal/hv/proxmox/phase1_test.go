package proxmox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"shippin.cloud/console/internal/hv"
)

// pve is a small stand-in with the JSON shapes the PVE 8 API returns.
type pve struct {
	mu        sync.Mutex
	guests    map[string]hv.Guest
	snaps     map[string][]string
	cfg       map[string]map[string]any
	tasks     map[string]int
	uploads   int
	downloads int
}

func newPVE() *pve {
	return &pve{
		guests: map[string]hv.Guest{
			"100": {ID: "100", Name: "app", Kind: hv.KindCT, Status: "running", Node: "n1"},
			"200": {ID: "200", Name: "db", Kind: hv.KindVM, Status: "stopped", Node: "n1"},
		},
		snaps: map[string][]string{},
		cfg:   map[string]map[string]any{"100": {"memory": "512"}, "200": {"cores": float64(2)}},
		tasks: map[string]int{},
	}
}

func (p *pve) handler(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "PVEAPIToken=") {
		http.Error(w, "auth", http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api2/json")
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case path == "/version":
		writeData(w, map[string]string{"version": "8.2.2", "release": "8.2"})
	case strings.HasSuffix(path, "/status/current"):
		id := strings.Split(path, "/")[4]
		for _, g := range p.guests {
			if g.ID == id {
				writeData(w, map[string]string{"status": g.Status})
				return
			}
		}
		http.NotFound(w, r)
	case path == "/cluster/resources":
		rows := []map[string]any{}
		for _, g := range p.guests {
			typ := "lxc"
			if g.Kind == hv.KindVM {
				typ = "qemu"
			}
			node := g.Node
			if node == "" {
				node = "n1"
			}
			rows = append(rows, map[string]any{"vmid": atoi(g.ID), "name": g.Name, "type": typ, "status": g.Status, "node": node, "tags": g.Tags})
		}
		writeData(w, rows)
	case strings.HasPrefix(path, "/nodes/n1/tasks/") && strings.HasSuffix(path, "/status"):
		upid := strings.TrimSuffix(strings.TrimPrefix(path, "/nodes/n1/tasks/"), "/status")
		p.tasks[upid]++
		st := "running"
		if p.tasks[upid] >= 2 {
			st = "stopped"
		}
		writeData(w, map[string]any{"upid": upid, "status": st, "exitstatus": "OK", "type": "qmcreate", "node": "n1"})
	case strings.HasPrefix(path, "/nodes/n1/tasks/") && strings.HasSuffix(path, "/log"):
		writeData(w, []map[string]any{{"n": 1, "t": "starting task"}, {"n": 2, "t": "TASK OK"}})
	case path == "/nodes/n1/services":
		writeData(w, []map[string]any{{"service": "pveproxy", "state": "running", "desc": "PVE API Proxy"}})
	case path == "/nodes/n1/network":
		writeData(w, []map[string]any{{"iface": "vmbr0", "type": "bridge"}, {"iface": "bond0", "type": "bond"}})
	case path == "/nodes/n1/firewall/rules":
		writeData(w, []map[string]any{{"type": "in", "action": "ACCEPT"}})
	case strings.HasSuffix(path, "/firewall/rules"):
		writeData(w, []map[string]any{{"type": "in", "action": "DROP"}})
	case strings.Contains(path, "/storage/local/content"):
		writeData(w, []map[string]any{{"volid": "local:iso/debian.iso", "content": "iso"}, {"volid": "pbs:backup/vm/200/2026-10-31", "content": "backup"}})
	case strings.HasSuffix(path, "/download-url"):
		p.downloads++
		writeData(w, "UPID:n1:00000001:00000002:00000003:download:local:token@pve:")
	case strings.HasSuffix(path, "/upload"):
		p.uploads++
		_, _ = io.Copy(io.Discard, r.Body)
		writeData(w, "UPID:n1:00000001:00000002:00000003:upload:local:token@pve:")
	case path == "/nodes/n1/vzdump":
		writeData(w, "UPID:n1:00000001:00000002:00000003:vzdump:200:token@pve:")
	default:
		p.guestRoutes(w, r, path)
	}
}

func (p *pve) guestRoutes(w http.ResponseWriter, r *http.Request, path string) {
	rest := strings.TrimPrefix(path, "/nodes/n1/")
	parts := strings.Split(rest, "/")
	if len(parts) == 1 && r.Method == http.MethodPost && (parts[0] == "qemu" || parts[0] == "lxc") {
		_ = r.ParseForm()
		id := r.FormValue("vmid")
		kind := hv.KindCT
		if parts[0] == "qemu" {
			kind = hv.KindVM
		}
		name := r.FormValue("name")
		if name == "" {
			name = r.FormValue("hostname")
		}
		p.guests[id] = hv.Guest{ID: id, Name: name, Kind: kind, Status: "stopped", Node: "n1"}
		writeData(w, "UPID:n1:00000001:00000002:00000003:qmcreate:"+id+":token@pve:")
		return
	}
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	kind, id := parts[0], parts[1]
	gkind := hv.KindCT
	if kind == "qemu" {
		gkind = hv.KindVM
	}
	tail := strings.Join(parts[2:], "/")
	switch {
	case r.Method == http.MethodPost && tail == "" && kind != "":
		_ = r.ParseForm()
		id = r.FormValue("vmid")
		p.guests[id] = hv.Guest{ID: id, Name: r.FormValue("name"), Kind: gkind, Status: "stopped", Node: "n1"}
		writeData(w, "UPID:n1:00000001:00000002:00000003:qmcreate:"+id+":token@pve:")
	case r.Method == http.MethodDelete && tail == "":
		delete(p.guests, id)
		writeData(w, "UPID:n1:00000001:00000002:00000003:qmdestroy:"+id+":token@pve:")
	case tail == "clone":
		_ = r.ParseForm()
		src := p.guests[id]
		nid := r.FormValue("newid")
		p.guests[nid] = hv.Guest{ID: nid, Name: r.FormValue("name"), Kind: src.Kind, Status: "stopped", Node: "n1"}
		writeData(w, "UPID:n1:00000001:00000002:00000003:qmclone:"+nid+":token@pve:")
	case tail == "template":
		g := p.guests[id]
		g.Status = "stopped"
		g.Tags = "template"
		p.guests[id] = g
		writeData(w, "UPID:n1:00000001:00000002:00000003:qmtemplate:"+id+":token@pve:")
	case tail == "config" && r.Method == http.MethodGet:
		writeData(w, p.cfg[id])
	case tail == "config" && r.Method == http.MethodPut:
		_ = r.ParseForm()
		if p.cfg[id] == nil {
			p.cfg[id] = map[string]any{}
		}
		for k, v := range r.Form {
			p.cfg[id][k] = v[0]
		}
		writeData(w, nil)
	case tail == "resize":
		writeData(w, "UPID:n1:00000001:00000002:00000003:resize:"+id+":token@pve:")
	case tail == "migrate":
		g := p.guests[id]
		_ = r.ParseForm()
		g.Node = r.FormValue("target")
		p.guests[id] = g
		writeData(w, "UPID:n1:00000001:00000002:00000003:qmmigrate:"+id+":token@pve:")
	case tail == "snapshot" && r.Method == http.MethodGet:
		rows := []map[string]string{{"name": "current"}}
		for _, n := range p.snaps[id] {
			rows = append(rows, map[string]string{"name": n})
		}
		writeData(w, rows)
	case tail == "snapshot" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		p.snaps[id] = append(p.snaps[id], r.FormValue("snapname"))
		writeData(w, "UPID:n1:00000001:00000002:00000003:snapshot:"+id+":token@pve:")
	case strings.HasSuffix(tail, "/rollback"):
		writeData(w, "UPID:n1:00000001:00000002:00000003:rollback:"+id+":token@pve:")
	default:
		http.NotFound(w, r)
	}
}

func TestPhase1AgainstRecordedShapes(t *testing.T) {
	p := newPVE()
	srv := httptest.NewTLSServer(http.HandlerFunc(p.handler))
	defer srv.Close()
	c := clientFor(t, srv)
	c.PollBase = time.Millisecond
	c.PollMax = 4 * time.Millisecond
	ctx := context.Background()

	task, err := c.Wait(ctx, "UPID:n1:00000001:00000002:00000003:qmstart:100:token@pve:")
	if err != nil || !task.Done || task.Exit != "OK" || len(task.Log) != 2 {
		t.Fatalf("wait %+v %v", task, err)
	}
	got, err := c.Create(ctx, Spec{Kind: hv.KindVM, ID: "201", Name: "new", Memory: "2048", Disk: "local-lvm:8"})
	if err != nil || got.ID != "201" || got.Kind != hv.KindVM {
		t.Fatalf("create %+v %v", got, err)
	}
	cloned, err := c.Clone(ctx, "100", Spec{NewID: "102", Name: "copy"})
	if err != nil || cloned.ID != "102" || cloned.Kind != hv.KindCT {
		t.Fatalf("clone %+v %v", cloned, err)
	}
	tpl, err := c.Template(ctx, "200")
	if err != nil || tpl.Tags != "template" {
		t.Fatalf("template %+v %v", tpl, err)
	}
	cfg, err := c.SetConfig(ctx, "100", map[string]string{"memory": "1024"})
	if err != nil || cfg["memory"] != "1024" {
		t.Fatalf("config %+v %v", cfg, err)
	}
	if _, err := c.Resize(ctx, "200", "scsi0", "+8G"); err != nil {
		t.Fatal(err)
	}
	moved, err := c.Migrate(ctx, "200", Spec{Target: "n2", Online: true})
	if err != nil || moved.Node != "n2" {
		t.Fatalf("migrate %+v %v", moved, err)
	}
	if _, err := c.Snapshot(ctx, "100", "before"); err != nil {
		t.Fatal(err)
	}
	names, err := c.Snapshots(ctx, "100")
	if err != nil || len(names) != 1 || names[0] != "before" {
		t.Fatalf("snaps %v %v", names, err)
	}
	if _, err := c.Rollback(ctx, "100", "before"); err != nil {
		t.Fatal(err)
	}
	vols, err := c.Content(ctx, "local", "iso")
	if err != nil || len(vols) != 2 {
		t.Fatalf("content %v %v", vols, err)
	}
	if _, err := c.DownloadURL(ctx, Spec{URL: "http://example.test/debian.iso", Content: "iso", File: "debian.iso"}); err == nil {
		t.Fatal("download must require https")
	}
	if _, err := c.DownloadURL(ctx, Spec{Storage: "local", URL: "https://example.test/debian.iso", Content: "iso", File: "debian.iso"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Upload(ctx, "local", "iso", "tiny.iso", strings.NewReader("iso")); err != nil {
		t.Fatal(err)
	}
	svc, err := c.Services(ctx)
	if err != nil || svc[0] != "pveproxy" {
		t.Fatalf("services %v %v", svc, err)
	}
	nets, err := c.NodeNetworks(ctx)
	if err != nil || nets[0].Name != "vmbr0" {
		t.Fatalf("nets %+v %v", nets, err)
	}
	fw, err := c.FirewallRules(ctx, "")
	if err != nil || fw[0] != "in ACCEPT" {
		t.Fatalf("fw %v %v", fw, err)
	}
	if _, err := c.Vzdump(ctx, Spec{ID: "200", Storage: "pbs", Mode: "snapshot"}); err != nil {
		t.Fatal(err)
	}
	backups, err := c.Backups(ctx, "local")
	if err != nil || !strings.Contains(backups[1], "pbs:backup") {
		t.Fatalf("backups %v %v", backups, err)
	}
	restored, err := c.Restore(ctx, Spec{Kind: hv.KindVM, ID: "202", Archive: "pbs:backup/vm/200/2026-10-31"})
	if err != nil || restored.ID != "202" {
		t.Fatalf("restore %+v %v", restored, err)
	}
	if err := c.Delete(ctx, "201"); err != nil {
		t.Fatal(err)
	}
	if p.uploads != 1 || p.downloads != 1 {
		t.Fatalf("upload %d download %d", p.uploads, p.downloads)
	}
}

func writeData(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
