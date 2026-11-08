package projects

import "net/http"

// Product workspace endpoints have a distinct name while the old /workspaces
// compute contract remains available to host agents and existing installations.
const workspaceBase = "/v1/tenants/{tenantId}/organization-workspaces"

func (h *Handler) listOrganizationWorkspaces(w http.ResponseWriter, r *http.Request, t, _ string) {
	out, err := h.S.OrganizationWorkspaces(r.Context(), t)
	reply(w, http.StatusOK, out, err)
}
func (h *Handler) getOrganizationWorkspace(w http.ResponseWriter, r *http.Request, t, _ string) {
	out, err := h.S.OrganizationWorkspace(r.Context(), t, r.PathValue("groupId"))
	reply(w, http.StatusOK, out, err)
}
func (h *Handler) createOrganizationWorkspace(w http.ResponseWriter, r *http.Request, t, actor string) {
	var in WorkspaceInput
	if !decode(w, r, &in) {
		return
	}
	id, err := h.S.CreateOrganizationWorkspace(r.Context(), t, actor, in)
	if err != nil {
		writeErr(w, err, "A workspace with that name already exists.")
		return
	}
	out, err := h.S.OrganizationWorkspace(r.Context(), t, id)
	reply(w, http.StatusCreated, out, err)
}
func (h *Handler) updateOrganizationWorkspace(w http.ResponseWriter, r *http.Request, t, _ string) {
	var in WorkspaceInput
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("groupId")
	if err := h.S.UpdateOrganizationWorkspace(r.Context(), t, id, in); err != nil {
		writeErr(w, err, "A workspace with that name already exists.")
		return
	}
	out, err := h.S.OrganizationWorkspace(r.Context(), t, id)
	reply(w, http.StatusOK, out, err)
}
func (h *Handler) deleteOrganizationWorkspace(w http.ResponseWriter, r *http.Request, t, _ string) {
	if err := h.S.DeleteOrganizationWorkspace(r.Context(), t, r.PathValue("groupId")); err != nil {
		writeErr(w, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) workspaceProjects(w http.ResponseWriter, r *http.Request, t, _ string) {
	out, err := h.S.WorkspaceProjects(r.Context(), t, r.PathValue("groupId"))
	reply(w, http.StatusOK, out, err)
}
