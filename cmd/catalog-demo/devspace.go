package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
	"shippin.cloud/console/internal/projects"
)

// mountDevspace serves the M1 projects API and a loopback stand-in for hostd workspaces for the
// one demo tenant named in the identity file (me.tenants[0]). Off unless CATALOG_DEMO_ENGINE is set.
func mountDevspace(mux *http.ServeMux, d *db.DB, identity []byte, eng *engines, vm *hostd.VMPlan) {
	var doc struct {
		Me struct {
			Tenants []struct {
				ID   string `json:"tenant_id"`
				Name string `json:"name"`
			} `json:"tenants"`
		} `json:"me"`
	}
	if json.Unmarshal(identity, &doc) != nil || len(doc.Me.Tenants) == 0 {
		log.Fatal("devspace demo: identity file needs me.tenants[0]")
	}
	t := doc.Me.Tenants[0]
	seedTenant(d, t.ID, t.Name, vm)
	own := func(h func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.PathValue("tenantId") != t.ID {
				auth.Problem(w, http.StatusNotFound, "Tenant not found.")
				return
			}
			h(w, r)
		}
	}
	ph := &projects.Handler{S: &projects.Store{Pool: d.Pool}}
	for _, rt := range ph.Routes() {
		fn := rt.H
		mux.HandleFunc(rt.Pattern, own(func(w http.ResponseWriter, r *http.Request) { fn(w, r, t.ID, "demo") }))
	}
	ws := &demoWS{d: d, t: t.ID, eng: eng}
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces", own(ws.list))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/workspaces", own(ws.create))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/workspaces/{workspaceId}/actions/{action}", own(ws.action))
	mux.HandleFunc("GET /v1/templates", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"items":[]}`)) })
}

func contextBG() context.Context { return context.Background() }

// seedTenant makes the demo tenant and its boundary #1 sized from hostd plan config
// (deploy/hostd.example.json plans.founder.vm), as the control plane does at tenant create (07c).
func seedTenant(d *db.DB, id, name string, vm *hostd.VMPlan) {
	ctx := context.Background()
	_, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan,status,ssh_ip) VALUES($1,'demo-org',$2,'founder','active','127.0.0.1')
	  ON CONFLICT (id) DO NOTHING`, id, name)
	if err != nil {
		log.Fatalf("seed tenant: %v", err)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM boundaries WHERE tenant_id=$1`, id).Scan(&n)
	if n == 0 {
		b, err := (&projects.Store{Pool: d.Pool}).PlanBoundary(ctx, id, vm.CPU, gibToMiB(vm.Memory), vm.RootGiB, vm.DataGiB)
		if err != nil {
			log.Fatalf("seed boundary: %v", err)
		}
		_, _ = d.Pool.Exec(ctx, `UPDATE boundaries SET state='ready' WHERE id=$1`, b)
	}
}

func gibToMiB(m string) int {
	n, err := strconv.Atoi(strings.TrimSuffix(m, "GiB"))
	if err != nil {
		log.Fatalf("plan memory %q: want <n>GiB", m)
	}
	return n * 1024
}
