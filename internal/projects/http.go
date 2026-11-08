package projects

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/auth"
)

// Handler serves the tenant-scoped projects API. The caller resolves and authorizes the tenant
// (console: tenant membership via tenantCtx; demo: one fixed tenant) and passes its id and actor.
type Handler struct{ S *Store }

type Route struct {
	Pattern string
	H       func(w http.ResponseWriter, r *http.Request, tenant, actor string)
}

var (
	projectName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,38}[a-z0-9]$`)
	envName     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,22}[a-z0-9]$`)
)

const base = "/v1/tenants/{tenantId}/projects"

func (h *Handler) Routes() []Route {
	return []Route{
		{"GET " + workspaceBase, h.listOrganizationWorkspaces},
		{"POST " + workspaceBase, h.createOrganizationWorkspace},
		{"GET " + workspaceBase + "/{groupId}", h.getOrganizationWorkspace},
		{"PATCH " + workspaceBase + "/{groupId}", h.updateOrganizationWorkspace},
		{"DELETE " + workspaceBase + "/{groupId}", h.deleteOrganizationWorkspace},
		{"GET " + workspaceBase + "/{groupId}/projects", h.workspaceProjects},
		{"GET " + base, h.list},
		{"POST " + base, h.create},
		{"GET " + base + "/{projectId}", h.get},
		{"DELETE " + base + "/{projectId}", h.remove},
		{"POST " + base + "/{projectId}/environments", h.addEnv},
		{"GET " + base + "/{projectId}/workspaces", h.workspaces},
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, t, _ string) {
	out, err := h.S.Projects(r.Context(), t)
	reply(w, http.StatusOK, out, err)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, t, _ string) {
	out, err := h.S.Project(r.Context(), t, r.PathValue("projectId"))
	reply(w, http.StatusOK, out, err)
}

func (h *Handler) workspaces(w http.ResponseWriter, r *http.Request, t, _ string) {
	out, err := h.S.Workspaces(r.Context(), t, r.PathValue("projectId"))
	reply(w, http.StatusOK, out, err)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dst) != nil {
		auth.Problem(w, http.StatusBadRequest, "The request body isn't valid JSON.")
		return false
	}
	return true
}

func reply(w http.ResponseWriter, code int, out json.RawMessage, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "Project not found.")
	case err != nil:
		auth.Problem(w, http.StatusInternalServerError, "Something went wrong.")
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_, _ = w.Write(out)
	}
}
