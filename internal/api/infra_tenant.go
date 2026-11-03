package api

import (
	"context"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/proxmox"
	"shippin.cloud/console/internal/servers"
)

// Tenant guest routes use the same guest model as the operator routes.
// A tenant sees a server it owns or is assigned. Estate rows with no assignment stay operator-only.
// Attaching credentials, creating, deleting, and migrating stay on the operator routes.
func (a *API) tenantHVRoutes(mux Router) {
	for _, rt := range []struct {
		pattern string
		mutate  bool
		h       func(http.ResponseWriter, *http.Request, *caller, string)
	}{
		{"GET /v1/tenants/{tenantId}/servers/{serverId}/health", false, a.tenantHealth},
		{"GET /v1/tenants/{tenantId}/servers/{serverId}/guests", false, a.tenantGuests},
		{"GET /v1/tenants/{tenantId}/servers/{serverId}/guests/{guest}", false, a.tenantGuest},
		{"POST /v1/tenants/{tenantId}/servers/{serverId}/guests/{guest}/status/{verb}", true, a.tenantStatus},
		{"GET /v1/tenants/{tenantId}/servers/{serverId}/guests/{guest}/snapshots", false, a.tenantSnapshots},
		{"POST /v1/tenants/{tenantId}/servers/{serverId}/guests/{guest}/snapshots", true, a.tenantSnapshot},
		{"POST /v1/tenants/{tenantId}/servers/{serverId}/guests/{guest}/snapshots/{name}/rollback", true, a.tenantRollback},
		{"GET /v1/tenants/{tenantId}/servers/{serverId}/tasks/{upid}", false, a.tenantTask},
	} {
		mux.HandleFunc(rt.pattern, a.flow.Require(a.tenantHV(rt.mutate, rt.h)))
	}
}

func (a *API) tenantHV(mutate bool, h func(http.ResponseWriter, *http.Request, *caller, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.Servers.Enabled && a.hvVisible == nil {
			auth.Problem(w, http.StatusNotFound, "Servers aren't enabled.")
			return
		}
		c, ok := a.resolve(w, r)
		if !ok {
			return
		}
		t, ok := c.tenant(w, r.PathValue("tenantId"))
		if !ok {
			return
		}
		if _, err := a.visibleEndpoint(r.Context(), t.ID, r.PathValue("serverId")); err != nil {
			auth.Problem(w, http.StatusNotFound, "This server is not available to this organization.")
			return
		}
		if mutate && !c.canDecide(t) {
			auth.Problem(w, http.StatusForbidden, "Starting and snapshotting guests needs the org owner or admin role.")
			return
		}
		h(w, r, c, t.ID)
	}
}

func (a *API) visibleEndpoint(ctx context.Context, tenant, id string) (servers.Endpoint, error) {
	if a.hvVisible != nil {
		return a.hvVisible(ctx, tenant, id)
	}
	if a.db == nil {
		return servers.Endpoint{}, servers.ErrNotFound
	}
	return a.serverStore().VisibleEndpoint(ctx, tenant, id)
}

func (a *API) tenantHealth(w http.ResponseWriter, r *http.Request, _ *caller, tenant string) {
	cl, err := a.tenantDriver(r.Context(), tenant, r.PathValue("serverId"))
	if err != nil {
		infraErr(w, err)
		return
	}
	h, err := cl.Health(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, h)
}

func (a *API) tenantGuests(w http.ResponseWriter, r *http.Request, _ *caller, tenant string) {
	cl, err := a.tenantDriver(r.Context(), tenant, r.PathValue("serverId"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := cl.Guests(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) tenantGuest(w http.ResponseWriter, r *http.Request, _ *caller, tenant string) {
	cl, err := a.tenantDriver(r.Context(), tenant, r.PathValue("serverId"))
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := cl.Guest(r.Context(), r.PathValue("guest"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) tenantDriver(ctx context.Context, tenant, id string) (hv.Driver, error) {
	ep, err := a.visibleEndpoint(ctx, tenant, id)
	if err != nil {
		return nil, err
	}
	return a.openDriver(ep)
}

func (a *API) tenantClient(ctx context.Context, tenant, id string) (*proxmox.Client, error) {
	ep, err := a.visibleEndpoint(ctx, tenant, id)
	if err != nil {
		return nil, err
	}
	return a.openEndpoint(ep)
}

func (a *API) tenantStatus(w http.ResponseWriter, r *http.Request, c *caller, tenant string) {
	verb := r.PathValue("verb")
	switch verb {
	case "start", "stop", "shutdown", "reboot":
	default:
		auth.Problem(w, http.StatusBadRequest, "Verb must be start, stop, shutdown, or reboot.")
		return
	}
	id, guest := r.PathValue("serverId"), r.PathValue("guest")
	if !a.permitFor(w, r, c, tenant, id, guest, verb) {
		return
	}
	cl, err := a.tenantDriver(r.Context(), tenant, id)
	if err != nil {
		infraErr(w, err)
		return
	}
	var (
		got  hv.Guest
		err2 error
	)
	switch verb {
	case "start":
		got, err2 = cl.Start(r.Context(), guest)
	case "stop":
		got, err2 = cl.Stop(r.Context(), guest)
	case "shutdown":
		got, err2 = cl.Shutdown(r.Context(), guest)
	default:
		got, err2 = cl.Reboot(r.Context(), guest)
	}
	if err2 != nil {
		infraErr(w, err2)
		return
	}
	writeJSON(w, got)
}

func (a *API) tenantSnapshots(w http.ResponseWriter, r *http.Request, _ *caller, tenant string) {
	cl, err := a.tenantClient(r.Context(), tenant, r.PathValue("serverId"))
	if err != nil {
		infraErr(w, err)
		return
	}
	names, err := cl.Snapshots(r.Context(), r.PathValue("guest"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": names})
}

func (a *API) tenantSnapshot(w http.ResponseWriter, r *http.Request, c *caller, tenant string) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	id, guest := r.PathValue("serverId"), r.PathValue("guest")
	if !a.permitFor(w, r, c, tenant, id, guest, "snapshot") {
		return
	}
	cl, err := a.tenantDriver(r.Context(), tenant, id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := cl.Snapshot(r.Context(), guest, in.Snap)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) tenantRollback(w http.ResponseWriter, r *http.Request, c *caller, tenant string) {
	id, guest, name := r.PathValue("serverId"), r.PathValue("guest"), r.PathValue("name")
	if !a.permitFor(w, r, c, tenant, id, guest, "rollback") {
		return
	}
	cl, err := a.tenantClient(r.Context(), tenant, id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := cl.Rollback(r.Context(), guest, name)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) tenantTask(w http.ResponseWriter, r *http.Request, _ *caller, tenant string) {
	cl, err := a.tenantClient(r.Context(), tenant, r.PathValue("serverId"))
	if err != nil {
		infraErr(w, err)
		return
	}
	task, err := cl.Task(r.Context(), r.PathValue("upid"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, task)
}
