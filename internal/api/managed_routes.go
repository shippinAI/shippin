package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"shippin.cloud/console/internal/db"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/managed"
)

// serviceRoutes: managed services attachable to a project's devspace (07-multi-tenancy), rrflow first.
func (a *API) serviceRoutes(mux Router) {
	base := "/v1/tenants/{tenantId}/projects/{projectId}/services"
	mux.HandleFunc("GET "+base, a.flow.Require(a.listServices))
	mux.HandleFunc("POST "+base+"/rrflow", a.flow.Require(a.attachRRFlowRoute))
	mux.HandleFunc("GET "+base+"/{serviceId}/rrflow/{kind}", a.flow.Require(a.rrflowState))
}

func (a *API) listServices(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !t.member() {
		auth.Problem(w, http.StatusForbidden, "Only members of this tenant can see its services.")
		return
	}
	if a.svcs == nil {
		writeJSON(w, map[string]any{"rrflow_enabled": false, "items": []managed.Instance{}})
		return
	}
	items, err := a.svcs.ByProject(r.Context(), t.ID, r.PathValue("projectId"))
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Services are unavailable right now.")
		return
	}
	if items == nil {
		items = []managed.Instance{}
	}
	writeJSON(w, map[string]any{"rrflow_enabled": a.cfg.Services.RRFlow.Allows(t.ID), "items": items})
}

// attachRRFlowRoute attaches rrflow/state to the project's devspace (or to workspace_id).
func (a *API) attachRRFlowRoute(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if a.svcs == nil || !a.cfg.Services.RRFlow.Allows(t.ID) {
		auth.Problem(w, http.StatusForbidden, "rrflow isn't available in this tenant yet.")
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Attaching services needs the org owner or admin role.")
		return
	}
	var in struct {
		WorkspaceID string `json:"workspace_id"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in)
	pid := r.PathValue("projectId")
	if in.WorkspaceID == "" {
		ws, _ := a.db.Workspaces(r.Context(), t.ID)
		if !hasProjectWorkspace(ws, pid) {
			auth.Problem(w, http.StatusConflict, "Open the project once so it has a devspace, then attach rrflow.")
			return
		}
		for _, x := range ws {
			if x.ProjectID == pid && x.State != "deleted" && x.State != "failed" {
				in.WorkspaceID = x.ID
				break
			}
		}
	}
	if ws, err := a.db.WorkspaceByID(r.Context(), in.WorkspaceID); err != nil || ws == nil || ws.TenantID != t.ID || ws.ProjectID != pid {
		auth.Problem(w, http.StatusNotFound, "That devspace isn't part of this project.")
		return
	}
	inst, j, err := a.queueRRFlow(r.Context(), t.ID, pid, in.WorkspaceID, c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "rrflow could not be attached right now.")
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]any{"instance": inst, "job": jobView(j, c.op != "")})
}

// queueRRFlow records the tenant VM's shared instance (D36), project's namespace in it and the
// workspace binding, then enqueues service.provision.
func (a *API) queueRRFlow(ctx context.Context, tenant, project, ws, by string) (*managed.Instance, *jobs.Job, error) {
	if err := a.rrflowVMAllowed(ctx, tenant); err != nil {
		return nil, nil, err
	}
	inst, err := a.svcs.EnsureVM(ctx, tenant, project, managed.KindRRFlow, by)
	if err != nil {
		return nil, nil, err
	}
	if _, err := a.svcs.Bind(ctx, tenant, inst.ID, ws, project); err != nil {
		return nil, nil, err
	}
	j := jobs.Job{TenantID: tenant, Kind: jobs.KindServiceProvision, TargetType: "service_instance", TargetID: inst.ID,
		MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: by}
	if a.runner == nil {
		go func() {
			if _, err := a.runServiceJob(context.Background(), &j, func(string) {}); err != nil {
				log.Printf("service %s: %v", inst.ID, err)
			}
		}()
		return inst, nil, nil
	}
	got, _, err := a.jobs.Enqueue(ctx, j)
	if err == nil {
		a.runner.Wake()
	}
	return inst, got, err
}

// errRRFlowHeld: the D36 rrflow schema (026) is still held by config migrations.hold.
var (
	errRRFlowHeld = errors.New("rrflow is off until migration 026 is applied")
	errRRFlowOff  = errors.New("rrflow is off for this tenant (services.rrflow enabled/tenants)")
	// errRRFlowLegacy: the tenant still runs pre-D36 per-project instances; they keep serving as they
	// are and no shared instance is created until the operator setup route moves the tenant.
	errRRFlowLegacy = errors.New("rrflow: this tenant still uses per-project instances until its move (operator setup)")
)

// rrflowStore is nil until the D36 schema (026_rrflow_per_vm.sql) is applied: managed services
// stay off while that migration is held (change ticket B), instead of failing on missing columns.
func rrflowStore(d *db.DB) *managed.Store {
	var ok bool
	if d.Pool.QueryRow(context.Background(), `SELECT to_regclass('service_namespaces') IS NOT NULL`).Scan(&ok) != nil || !ok {
		log.Printf("managed services off: rrflow schema (026) not applied")
		return nil
	}
	return &managed.Store{Pool: d.Pool}
}
