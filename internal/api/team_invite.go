package api

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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
		auth.Problem(w, http.StatusBadGateway, a.idpWord()+" refused the invite (the e-mail may already be in use).")
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

// inviteLinkFor is how long an invite link lasts.
const inviteLinkFor = 7 * 24 * time.Hour

// teamInviteLink makes a one-time link for someone invited on this panel, for them to claim their
// account. An identity provider that holds staff sends its own invitations.
func (a *API) teamInviteLink(w http.ResponseWriter, r *http.Request, c *caller) {
	local, ok := a.idp().(*localStaff)
	if !ok || a.directory == nil {
		auth.Problem(w, http.StatusConflict, a.idpWord()+" sends its own invitations.")
		return
	}
	id := r.PathValue("userId")
	u, err := local.User(r.Context(), id)
	if err != nil || u == nil {
		auth.Problem(w, http.StatusNotFound, "That person has no account here.")
		return
	}
	if claimed, err := local.Claimed(r.Context(), id); err != nil || claimed {
		auth.Problem(w, http.StatusConflict, "They have already signed in, so a link would do nothing.")
		return
	}
	if !a.auditFirst(w, r, teamEntry(r, c, "team.invite_link", u.Name+" ("+id+")")) {
		return
	}
	actor, _ := localID(c.id.Sub)
	code, err := a.directory.CreateInvite(r.Context(), actor, u.Email, "member", 1, time.Now().Add(inviteLinkFor))
	if err != nil {
		log.Printf("team invite link for %s: %v", id, err)
		auth.Problem(w, http.StatusServiceUnavailable, "The link could not be made.")
		return
	}
	writeJSON(w, map[string]string{"url": "https://" + r.Host + "/invite?code=" + url.QueryEscape(code) + "&email=" + url.QueryEscape(u.Email)})
}

// idpWord names the identity provider in a message, whether or not it has been given a name.
func (a *API) idpWord() string {
	if a.cfg.IDP.Name != "" {
		return a.cfg.IDP.Name
	}
	return "The identity provider"
}
