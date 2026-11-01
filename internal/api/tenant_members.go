package api

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// Tenant Team page: Nomen org members + org roles, and Shippin teams. Reads: any member.
// Writes: owners/admins (canDecide), audit-first, then Nomen; owner-role changes owners only.

func (a *API) tenantTeamRoutes(mux Router) {
	for p, h := range a.tenantTeamHandlers() {
		mux.HandleFunc(p, a.flow.Require(h))
	}
}

// tenantTeamRoutesDirect mounts the handlers without the session wrapper (tests).
func (a *API) tenantTeamRoutesDirect(mux Router) {
	for p, h := range a.tenantTeamHandlers() {
		mux.HandleFunc(p, h)
	}
}

func (a *API) tenantTeamHandlers() map[string]http.HandlerFunc {
	b := "/v1/tenants/{tenantId}"
	return map[string]http.HandlerFunc{
		"GET " + b + "/team": a.tenantTeam, "GET " + b + "/members": a.listTenantMembers,
		"POST " + b + "/members": a.inviteTenantMember, "PATCH " + b + "/members/{userId}": a.setTenantMemberRoles,
		"DELETE " + b + "/members/{userId}": a.removeTenantMember, "POST " + b + "/teams": a.createTeam,
		"DELETE " + b + "/teams/{teamId}": a.deleteTeam, "PUT " + b + "/teams/{teamId}/members/{userId}": a.setTeamMember,
		"DELETE " + b + "/teams/{teamId}/members/{userId}": a.removeTeamMember,
	}
}

// teamCtx resolves the tenant; write=true also requires canDecide.
func (a *API) teamCtx(w http.ResponseWriter, r *http.Request, write bool) (*caller, *tenantView, bool) {
	c, ok := a.resolve(w, r)
	if !ok {
		return nil, nil, false
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return nil, nil, false
	}
	if !t.member() {
		auth.Problem(w, http.StatusForbidden, "Only members of this tenant can see its team.")
		return nil, nil, false
	}
	if write && !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Managing the team needs the owner or admin role.")
		return nil, nil, false
	}
	return c, t, true
}

func (a *API) tenantTeam(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, false)
	if !ok {
		return
	}
	members, err := a.orgIdP().Members(r.Context(), t.IdpOrgID)
	errs := []string{}
	if err != nil {
		log.Printf("tenant %s members: %v", t.ID, err)
		members, errs = []auth.OrgMember{}, append(errs, a.cfg.IDP.Name+" members are unavailable right now.")
	}
	teams, err := a.db.Teams(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Teams are unavailable right now.")
		return
	}
	if members == nil {
		members = []auth.OrgMember{}
	}
	if teams == nil {
		teams = []db.Team{}
	}
	writeJSON(w, map[string]any{"members": members, "teams": teams, "roles": a.assignableRoles(), "owner_role": a.ownerRole(),
		"can_manage": c.canDecide(t), "is_owner": slices.Contains(t.Roles, a.cfg.Roles.Owner), "errors": errs})
}

// listTenantMembers is the spec's listMembers (paths/members.yaml): the org members only.
func (a *API) listTenantMembers(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.teamCtx(w, r, false)
	if !ok {
		return
	}
	ms, err := a.orgIdP().Members(r.Context(), t.IdpOrgID)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" members are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": ms})
}

func (a *API) teamAudit(r *http.Request, c *caller, t *tenantView, action, detail string) db.AuditEntry {
	return db.AuditEntry{TenantID: t.ID, TenantName: t.Name, Actor: c.id.Sub, ActorName: displayName(c), Action: action, Detail: detail, IP: clientIP(r)}
}

func (a *API) inviteTenantMember(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Email     string   `json:"email"`
		FirstName string   `json:"first_name"`
		LastName  string   `json:"last_name"`
		Roles     []string `json:"roles"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || !emailRe.MatchString(strings.TrimSpace(in.Email)) || len(in.Roles) == 0 {
		auth.Problem(w, http.StatusBadRequest, "Send an e-mail address and at least one role.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if p := roleChangeProblem(a.assignableRoles(), a.ownerRole(), slices.Contains(t.Roles, a.cfg.Roles.Owner), nil, in.Roles, 2); p != "" {
		auth.Problem(w, http.StatusForbidden, p)
		return
	}
	idp, ctx := a.orgIdP(), r.Context()
	uid, err := idp.UserByEmail(ctx, email)
	first, last := strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName)
	if err == nil && uid == "" && (first == "" || last == "") {
		auth.Problem(w, http.StatusUnprocessableEntity, "No account has this e-mail yet; add a first and last name to invite them.")
		return
	}
	if !a.auditFirst(w, r, a.teamAudit(r, c, t, "tenant.member_invite", email+" roles "+strings.Join(in.Roles, ","))) {
		return
	}
	if err == nil && uid == "" {
		uid, err = idp.Invite(ctx, t.IdpOrgID, email, first, last)
	}
	if err == nil {
		err = idp.SetMember(ctx, t.IdpOrgID, uid, in.Roles, true)
	}
	if err != nil {
		log.Printf("tenant %s invite: %v", t.ID, err)
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" refused the invite (already a member?).")
		return
	}
	writeStatus(w, http.StatusCreated, map[string]any{"user_id": uid, "roles": in.Roles})
}
