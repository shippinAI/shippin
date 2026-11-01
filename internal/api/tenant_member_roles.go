package api

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"

	"shippin.cloud/console/internal/auth"
)

// memberState loads the org's members and finds userId among them.
func (a *API) memberState(w http.ResponseWriter, r *http.Request, t *tenantView) (before []string, owners int, ok bool) {
	ms, err := a.orgIdP().Members(r.Context(), t.IdpOrgID)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" members are unavailable right now.")
		return nil, 0, false
	}
	found, owner := false, a.ownerRole()
	for _, m := range ms {
		if slices.Contains(m.Roles, owner) {
			owners++
		}
		if m.UserID == r.PathValue("userId") {
			before, found = m.Roles, true
		}
	}
	if !found {
		auth.Problem(w, http.StatusNotFound, "That person isn't a member of this tenant.")
		return nil, 0, false
	}
	return before, owners, true
}

func (a *API) setTenantMemberRoles(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Roles []string `json:"roles"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in) != nil || len(in.Roles) == 0 {
		auth.Problem(w, http.StatusBadRequest, "Send at least one role (remove the member to take all).")
		return
	}
	before, owners, ok := a.memberState(w, r, t)
	if !ok {
		return
	}
	if p := roleChangeProblem(a.assignableRoles(), a.ownerRole(), slices.Contains(t.Roles, a.cfg.Roles.Owner), before, in.Roles, owners); p != "" {
		auth.Problem(w, http.StatusForbidden, p)
		return
	}
	uid := r.PathValue("userId")
	if !a.auditFirst(w, r, a.teamAudit(r, c, t, "tenant.member_roles", uid+" "+strings.Join(before, ",")+" -> "+strings.Join(in.Roles, ","))) {
		return
	}
	if err := a.orgIdP().SetMember(r.Context(), t.IdpOrgID, uid, in.Roles, false); err != nil {
		log.Printf("tenant %s roles %s: %v", t.ID, uid, err)
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" refused the role change.")
		return
	}
	writeJSON(w, map[string]any{"user_id": uid, "roles": in.Roles})
}

func (a *API) removeTenantMember(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, true)
	if !ok {
		return
	}
	before, owners, ok := a.memberState(w, r, t)
	if !ok {
		return
	}
	if p := roleChangeProblem(a.assignableRoles(), a.ownerRole(), slices.Contains(t.Roles, a.cfg.Roles.Owner), before, nil, owners); p != "" {
		auth.Problem(w, http.StatusForbidden, p)
		return
	}
	uid := r.PathValue("userId")
	if !a.auditFirst(w, r, a.teamAudit(r, c, t, "tenant.member_remove", uid)) {
		return
	}
	if err := a.orgIdP().RemoveMember(r.Context(), t.IdpOrgID, uid); err != nil {
		log.Printf("tenant %s remove %s: %v", t.ID, uid, err)
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" refused the removal.")
		return
	}
	if err := a.db.RemoveFromAllTeams(r.Context(), t.ID, uid); err != nil {
		log.Printf("tenant %s teams purge %s: %v", t.ID, uid, err)
	}
	w.WriteHeader(http.StatusNoContent)
}
