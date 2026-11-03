package api

import (
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"shippin.cloud/console/internal/auth"
)

var emailRe = regexp.MustCompile(`^[^@\s]{1,64}@[A-Za-z0-9.-]{1,190}\.[A-Za-z]{2,24}$`)

type rolesIn struct {
	Roles  []string `json:"roles"`
	Reason string   `json:"reason"`
}

// teamSetRoles replaces a staff member's roles. Invariants: no self-grant, the last Owner
// stays, operator-org users only, audit before the Nomen write, two-person when active.
func (a *API) teamSetRoles(w http.ResponseWriter, r *http.Request, c *caller) {
	var in rolesIn
	if !decodeStrict(w, r, &in) || !reasonOK(w, in.Reason) {
		return
	}
	in.Roles = uniq(in.Roles)
	id := r.PathValue("userId")
	u, err := a.idp().User(r.Context(), id)
	if err != nil || u == nil {
		auth.Problem(w, http.StatusNotFound, "Staff member not found in the operator organization.")
		return
	}
	cur, err := a.idp().Grant(r.Context(), id)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, a.idpWord()+" couldn't be read.")
		return
	}
	roles, ok := a.checkRoles(w, r, in.Roles)
	if !ok {
		return
	}
	added := slices.DeleteFunc(slices.Clone(in.Roles), func(k string) bool { return slices.Contains(cur.Roles, k) })
	if id == c.id.Sub && len(added) > 0 {
		a.auditOnce(r, c, "authz.denied", "team.write", "self-grant refused")
		auth.Problem(w, http.StatusForbidden, "You can't give yourself roles; ask another Owner.")
		return
	}
	if n, err := a.ownersAfter(r.Context(), map[string][]string{id: in.Roles}, nil, ""); err != nil {
		auth.Problem(w, http.StatusBadGateway, "Owners couldn't be counted, so nothing changed.")
		return
	} else if n == 0 {
		auth.Problem(w, http.StatusConflict, "That would leave no active Owner. Make someone else an Owner first.")
		return
	}
	if a.twoPersonFor(r, permsFor(roles, added)) {
		a.queueRequest(w, r, c, "roles", id, u.Name, in)
		return
	}
	a.applyRoles(w, r, c, id, u.Name, cur, in)
}

// applyRoles audits, then writes the grant to Nomen; a failed write is audited too.
func (a *API) applyRoles(w http.ResponseWriter, r *http.Request, c *caller, id, name string, cur staffGrant, in rolesIn) {
	detail := name + " (" + id + "): [" + strings.Join(cur.Roles, ", ") + "] → [" + strings.Join(in.Roles, ", ") + "] · " + in.Reason
	if !a.auditFirst(w, r, teamEntry(r, c, "team.roles", detail)) {
		return
	}
	if err := a.idp().SetRoles(r.Context(), id, cur, in.Roles); err != nil {
		log.Printf("team roles for %s: %v", id, err)
		_ = a.store().AddAudit(r.Context(), teamEntry(r, c, "team.roles_failed", name+" ("+id+"): "+a.idpWord()+" refused the change"))
		auth.Problem(w, http.StatusBadGateway, a.idpWord()+" refused the change; nothing was granted.")
		return
	}
	a.staffCache.flush()
	writeJSON(w, map[string]any{"id": id, "roles": in.Roles, "status": "applied"})
}

// teamSetActive deactivates or reactivates a staff member (never yourself, never the last Owner).
func (a *API) teamSetActive(w http.ResponseWriter, r *http.Request, c *caller) {
	state := r.PathValue("state")
	var in struct {
		Reason string `json:"reason"`
	}
	if (state != "deactivate" && state != "reactivate") || !decodeStrict(w, r, &in) || !reasonOK(w, in.Reason) {
		if state != "deactivate" && state != "reactivate" {
			auth.Problem(w, http.StatusNotFound, "Unknown action.")
		}
		return
	}
	id := r.PathValue("userId")
	u, err := a.idp().User(r.Context(), id)
	if err != nil || u == nil {
		auth.Problem(w, http.StatusNotFound, "Staff member not found in the operator organization.")
		return
	}
	if id == c.id.Sub {
		auth.Problem(w, http.StatusConflict, "You can't change your own account state here.")
		return
	}
	if state == "deactivate" {
		if n, err := a.ownersAfter(r.Context(), map[string][]string{id: nil}, nil, ""); err != nil || n == 0 {
			auth.Problem(w, http.StatusConflict, "That would leave no active Owner.")
			return
		}
	}
	if !a.auditFirst(w, r, teamEntry(r, c, "team."+state, u.Name+" ("+id+") · "+in.Reason)) {
		return
	}
	if err := a.idp().SetActive(r.Context(), id, state == "reactivate"); err != nil {
		log.Printf("team %s %s: %v", state, id, err)
		_ = a.store().AddAudit(r.Context(), teamEntry(r, c, "team."+state+"_failed", u.Name+" ("+id+")"))
		auth.Problem(w, http.StatusBadGateway, a.idpWord()+" refused the change.")
		return
	}
	a.staffCache.flush()
	writeJSON(w, map[string]string{"id": id, "status": state + "d"})
}
