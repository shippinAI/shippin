package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/projects"
)

// nomenSubRe: Nomen user ids are numeric (also keeps the IdP path clean).
var nomenSubRe = regexp.MustCompile(`^[0-9]{6,32}$`)

// opCreateProject is the platform-operator create (staff perm projects.create, audited): the same
// discover -> createOwned path a tenant's Adopt click uses, with the owner named explicitly (a Nomen
// user id that must belong to the tenant's Nomen org; the devspace is linked only if that user
// owns it) and placement/limits/cost_center settable. Never acts through the tenant's session.
func (a *API) opCreateProject(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	var in struct {
		WorkspaceID string          `json:"workspace_id"`
		Path        string          `json:"path"`
		OwnerSub    string          `json:"owner_sub"`
		Placement   string          `json:"placement"`
		CostCenter  string          `json:"cost_center"`
		Limits      projects.Limits `json:"limits"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil || in.WorkspaceID == "" || in.Path == "" || in.OwnerSub == "" {
		auth.Problem(w, http.StatusBadRequest, "workspace_id, path and owner_sub are required.")
		return
	}
	if !nomenSubRe.MatchString(in.OwnerSub) || a.orgMember(r.Context(), t.IdpOrgID, in.OwnerSub) != nil {
		auth.Problem(w, http.StatusForbidden, "owner_sub is not a member of this tenant's Nomen org.")
		return
	}
	a.idempotent(w, r, "op:"+t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) {
		f, err := a.pickFound(r.Context(), t.ID, in.WorkspaceID, in.Path)
		if err != nil {
			auth.Problem(w, http.StatusNotFound, "That folder is not a suggested project in the devspace.")
			return
		}
		a.finishOwned(w, r, ownedReq{TenantID: t.ID, WorkspaceID: in.WorkspaceID, OwnerSub: in.OwnerSub, By: c.id.Sub,
			Found: f, Placement: in.Placement, CostCenter: in.CostCenter, Limits: in.Limits},
			db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: in.WorkspaceID, Actor: c.id.Sub,
				ActorName: c.id.Name, Action: "project.create.operator", IP: clientIP(r)})
	})
}

// opDiscover lets the operator see the same suggestions the tenant sees (read-only). It shows
// folder names and git remotes, so it is gated on projects.create, not workspaces.read.
func (a *API) opDiscover(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	items, err := a.host.Discover(r.Context(), t.ID, r.PathValue("workspaceId"))
	if err != nil {
		auth.Problem(w, http.StatusConflict, "Devspace is not running.")
		return
	}
	writeJSON(w, map[string]any{"items": items})
}
