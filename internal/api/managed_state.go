package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/managed"
	"shippin.cloud/console/internal/rrflow/state"
)

// rrflowState reads sessions, memory or the decision log of the project's rrflow instance from the
// tenant VM, decoded into rrflow/state's own types (guest output is never passed through raw).
func (a *API) rrflowState(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !t.member() {
		auth.Problem(w, http.StatusForbidden, "Only members of this tenant can read rrflow.")
		return
	}
	if a.svcs == nil {
		auth.Problem(w, http.StatusNotFound, "No rrflow here.")
		return
	}
	inst, err := a.svcs.Get(r.Context(), t.ID, r.PathValue("serviceId"))
	pid := r.PathValue("projectId")
	if err != nil || inst.Kind != managed.KindRRFlow || !(inst.ProjectID == pid || (inst.Scope == "vm" && a.svcs.HasNamespace(r.Context(), inst.ID, pid))) {
		auth.Problem(w, http.StatusNotFound, "No rrflow here.")
		return
	}
	kind := r.PathValue("kind")
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	raw, err := a.host.ServiceState(ctx, hostclient.Svc{Tenant: t.ID, Project: pid, ID: inst.ID, Shared: inst.Scope == "vm"}, kind, n)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "rrflow didn't answer. It may still be starting.")
		return
	}
	var out any
	switch kind {
	case "sessions":
		v := []state.Session{}
		err = json.Unmarshal(raw, &v)
		out = map[string]any{"items": orEmpty(v)}
	case "memory":
		v := []state.MemoryItem{}
		err = json.Unmarshal(raw, &v)
		out = map[string]any{"items": orEmpty(v)}
	case "decisions":
		v := &struct {
			Items   []state.Decision `json:"items"`
			ChainOK bool             `json:"chain_ok"`
		}{}
		err = json.Unmarshal(raw, v)
		v.Items = orEmpty(v.Items)
		out = v
	default:
		auth.Problem(w, http.StatusNotFound, "Unknown rrflow view.")
		return
	}
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "rrflow sent something unexpected.")
		return
	}
	writeJSON(w, out)
}

// orEmpty keeps "items" a JSON array (never null) for the console.
func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
