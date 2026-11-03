package api

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// teamRoutes is the Team section: staff, roles (data), settings and two-person requests.
func (a *API) teamRoutes() []opRoute {
	return []opRoute{
		{"GET /v1/operator/team", "team.read", a.teamList},
		{"POST /v1/operator/team/members", "team.write", a.teamInvite},
		{"PUT /v1/operator/team/members/{userId}/roles", "team.write", a.teamSetRoles},
		{"POST /v1/operator/team/members/{userId}/{state}", "team.write", a.teamSetActive},
		{"POST /v1/operator/team/roles", "team.roles", a.teamCreateRole},
		{"PUT /v1/operator/team/roles/{roleKey}", "team.roles", a.teamUpdateRole},
		{"DELETE /v1/operator/team/roles/{roleKey}", "team.roles", a.teamDeleteRole},
		{"PUT /v1/operator/team/settings", "team.settings", a.teamSaveSettings},
		{"POST /v1/operator/team/requests/{requestId}/{decision}", "team.write", a.teamDecideRequest},
	}
}

type teamMember struct {
	staffUser
	Roles    []string   `json:"roles"`
	MFA      bool       `json:"mfa"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
	Idle     bool       `json:"idle"`
	Owner    bool       `json:"owner"`
	Self     bool       `json:"self"`
}

// teamList is everything the Team page shows. Humans of the operator org appear even without
// roles (so invited people can be given some); machine users only when they hold a role.
func (a *API) teamList(w http.ResponseWriter, r *http.Request, c *caller) {
	ctx := r.Context()
	roles, err := a.store().StaffRoles(ctx)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Staff roles are unavailable right now.")
		return
	}
	users, uerr := a.idp().Users(ctx)
	grants, gerr := a.idp().AllGrants(ctx)
	seen, _ := a.store().StaffSeen(ctx)
	reqs, _ := a.store().StaffRequests(ctx)
	s := a.teamSettings(ctx)
	members, owners := []teamMember{}, 0
	for _, u := range users {
		keys := grants[u.ID].Roles
		if u.Kind != "human" && len(keys) == 0 {
			continue
		}
		m := teamMember{staffUser: u, Roles: append([]string{}, keys...), Self: u.ID == c.id.Sub}
		m.Owner = permsFor(roles, keys)[permOwner]
		if len(keys) > 0 {
			m.MFA, _ = a.idp().MFA(ctx, u.ID)
		}
		if t, ok := seen[u.ID]; ok {
			m.LastSeen = &t
		}
		m.Idle = len(keys) > 0 && s.IdleDays > 0 && (m.LastSeen == nil || time.Since(*m.LastSeen) > time.Duration(s.IdleDays)*24*time.Hour)
		if m.Owner && u.State == "active" {
			owners++
		}
		members = append(members, m)
	}
	slices.SortStableFunc(members, func(x, y teamMember) int { return len(y.Roles) - len(x.Roles) })
	errs := []string{}
	if uerr != nil || gerr != nil {
		log.Printf("team list: users=%v grants=%v", uerr, gerr)
		errs = append(errs, a.cfg.IDP.Name+" couldn't be read; the staff list may be incomplete.")
	}
	writeJSON(w, map[string]any{"members": members, "roles": roles, "catalog": permCatalog, "settings": s,
		"requests": reqs, "owners": owners, "two_person_active": s.TwoPerson && owners >= 2,
		"mode_override": a.cfg.Staff.Mode, "self": c.id.Sub, "errors": errs})
}

// decodeStrict reads a small JSON body, refusing unknown fields.
func decodeStrict(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
		return false
	}
	return true
}

// teamEntry is an audit entry for a team change (no tenant).
func teamEntry(r *http.Request, c *caller, action, detail string) db.AuditEntry {
	return db.AuditEntry{Actor: c.id.Sub, ActorName: displayName(c), Action: action, Detail: detail, IP: clientIP(r)}
}

// auditFirst writes the entry before any IdP write; if it fails, nothing is changed.
// auditOperator records one operator action before it runs (04-build-plan gate: every operator
// action is audited). Fail-closed like auditFirst: no entry, no action.
func (a *API) auditOperator(w http.ResponseWriter, r *http.Request, c *caller, tenantID, action, detail string) bool {
	return a.auditFirst(w, r, db.AuditEntry{TenantID: tenantID, Actor: c.id.Sub, ActorName: displayName(c),
		Action: action, Detail: detail, IP: clientIP(r)})
}

func (a *API) auditFirst(w http.ResponseWriter, r *http.Request, e db.AuditEntry) bool {
	if err := a.store().AddAudit(r.Context(), e); err != nil {
		log.Printf("audit %s: %v", e.Action, err)
		auth.Problem(w, http.StatusServiceUnavailable, "The change couldn't be audited, so it wasn't made.")
		return false
	}
	return true
}
