package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/projects"
)

// opTenantVM: the tenant VM (size, usage, nested containers) or mode "legacy".
func (a *API) opTenantVM(w http.ResponseWriter, r *http.Request, _ *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	v, err := a.host.TenantVM(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "Tenant VM unavailable: "+err.Error())
		return
	}
	writeJSON(w, v)
}

// opEnableTenantVM makes a legacy tenant VM-backed. Existing workspaces are not moved.
func (a *API) opEnableTenantVM(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	if t.Status != "active" && t.Status != "failed" {
		auth.Problem(w, http.StatusConflict, "Tenant is "+t.Status+".")
		return
	}
	if !a.auditOperator(w, r, c, t.ID, "tenant.vm.enable", t.Plan) {
		return
	}
	log.Printf("operator %s: enable tenant VM for %s", c.id.Sub, t.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	v, err := a.host.EnableTenantVM(ctx, t.ID, t.Plan)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "Tenant VM setup failed: "+err.Error())
		return
	}
	if v.Mode == "vm" && v.Name != "" {
		if _, err := (&projects.Store{Pool: a.db.Pool}).AdoptVM(ctx, t.ID, v.Name, v.CPU, v.MemLimit, v.RootSize, v.DataSize); err != nil {
			log.Printf("tenant %s: boundary row for %s not recorded: %v", t.ID, v.Name, err)
		}
	}
	writeJSON(w, v)
}

var vmActions = map[string]bool{"start": true, "stop": true, "restart": true}

// opTenantVMAction starts/stops/restarts the tenant VM, then records its nested workspaces' states.
func (a *API) opTenantVMAction(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	act := r.PathValue("action")
	if !vmActions[act] {
		auth.Problem(w, http.StatusBadRequest, "Unknown action.")
		return
	}
	if act != "stop" && t.Status != "active" {
		auth.Problem(w, http.StatusConflict, "Tenant is "+t.Status+".")
		return
	}
	if !a.auditOperator(w, r, c, t.ID, "tenant.vm."+act, "") {
		return
	}
	log.Printf("operator %s: tenant VM %s %s", c.id.Sub, t.ID, act)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	before, _ := a.host.TenantVM(ctx, t.ID) // nested list is only readable while the VM runs
	if err := a.host.VMAction(ctx, t.ID, act); err != nil {
		auth.Problem(w, http.StatusConflict, "Tenant VM "+act+" failed: "+err.Error())
		return
	}
	v, err := a.host.TenantVM(ctx, t.ID)
	if err == nil {
		a.recordNested(ctx, t.ID, before, v)
	}
	writeJSON(w, map[string]any{"status": "ok", "vm": v})
}

// recordNested mirrors nested container states into the workspace rows after a VM action.
// The workspace set is the union of what ran in the VM before and after the action.
// Ids reported by the guest are only applied to this tenant's own workspace rows.
func (a *API) recordNested(ctx context.Context, tenant string, before, after *hostclient.TenantVM) {
	state := nestedStates(before, after)
	own, err := a.db.Workspaces(ctx, tenant)
	if err != nil {
		return
	}
	for _, w := range own {
		if st, ok := state[w.ID]; ok {
			_ = a.db.SetWorkspaceState(ctx, w.ID, st, "", "")
		}
	}
}

func nestedStates(before, after *hostclient.TenantVM) map[string]string {
	state := map[string]string{}
	for _, v := range []*hostclient.TenantVM{before, after} {
		if v == nil {
			continue
		}
		for _, c := range v.Containers {
			if c.Workspace != "" {
				state[c.Workspace] = "stopped"
			}
		}
	}
	if after != nil && after.Status == "Running" {
		for _, c := range after.Containers {
			if c.Workspace != "" && c.Status == "Running" {
				state[c.Workspace] = "running"
			}
		}
	}
	return state
}
