package api

import (
	"log"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
)

type inviteIn struct {
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Roles     []string `json:"roles"`
	Reason    string   `json:"reason"`
}

// teamInvite adds anyone by e-mail to the operator org (Nomen sends the sign-up link) and
// optionally grants roles. High-risk roles go through the two-person rule when it is active.
func (a *API) teamInvite(w http.ResponseWriter, r *http.Request, c *caller) {
	var in inviteIn
	if !decodeStrict(w, r, &in) || !reasonOK(w, in.Reason) {
		return
	}
	in.Email, in.FirstName, in.LastName = strings.ToLower(strings.TrimSpace(in.Email)), strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName)
	in.Roles = uniq(in.Roles)
	if !emailRe.MatchString(in.Email) {
		auth.Problem(w, http.StatusBadRequest, "Enter a valid e-mail address.")
		return
	}
	if in.FirstName == "" || in.LastName == "" || len(in.FirstName) > 60 || len(in.LastName) > 60 {
		auth.Problem(w, http.StatusBadRequest, "First and last name are required (up to 60 characters each).")
		return
	}
	roles, ok := a.checkRoles(w, r, in.Roles)
	if !ok {
		return
	}
	if a.twoPersonFor(r, permsFor(roles, in.Roles)) {
		a.queueRequest(w, r, c, "invite", in.Email, in.FirstName+" "+in.LastName, in)
		return
	}
	a.applyInvite(w, r, c, in)
}

// applyInvite: audit, create the user, then (audited again) grant the roles.
func (a *API) applyInvite(w http.ResponseWriter, r *http.Request, c *caller, in inviteIn) {
	name := in.FirstName + " " + in.LastName
	if !a.auditFirst(w, r, teamEntry(r, c, "team.invite", name+" <"+in.Email+"> with ["+strings.Join(in.Roles, ", ")+"] · "+in.Reason)) {
		return
	}
	id, err := a.idp().Invite(r.Context(), in.Email, in.FirstName, in.LastName)
	if err != nil || id == "" {
		log.Printf("team invite %s: %v", in.Email, err)
		_ = a.store().AddAudit(r.Context(), teamEntry(r, c, "team.invite_failed", in.Email))
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" refused the invite (the e-mail may already be in use).")
		return
	}
	if len(in.Roles) == 0 {
		a.staffCache.flush()
		writeStatus(w, http.StatusCreated, map[string]any{"id": id, "roles": in.Roles, "status": "invited"})
		return
	}
	w.Header().Set("X-Staff-Invited", id)
	a.applyRoles(w, r, c, id, name, staffGrant{}, rolesIn{Roles: in.Roles, Reason: "invite: " + in.Reason})
}
