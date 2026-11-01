package api

import (
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// deciderCtx: tenant owners/admins (members only; a grant holder can never decide).
func (a *API) deciderCtx(w http.ResponseWriter, r *http.Request) (*caller, *tenantView, bool) {
	c, t, ok := a.tenantCtx(w, r)
	if ok && !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Only the organization's owners and admins manage platform access.")
		return nil, nil, false
	}
	return c, t, ok
}

// tenantAccess lists the tenant's requests and grants plus break-glass notices for the banner.
// Legal-hold break-glass grants stay hidden until their notice time.
func (a *API) tenantAccess(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.deciderCtx(w, r)
	if !ok {
		return
	}
	gs, err := a.db.TenantGrants(r.Context(), t.ID, true, 100)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Access requests are unavailable right now.")
		return
	}
	gs = present(gs)
	notices := []db.Grant{}
	window := time.Duration(a.cfg.Access.BreakGlass.NoticeHours) * time.Hour
	for _, g := range gs {
		if g.Kind == "breakglass" && g.NoticeAt != nil && time.Since(*g.NoticeAt) < window {
			notices = append(notices, g)
		}
	}
	writeJSON(w, map[string]any{"items": gs, "notices": notices})
}

var decisions = map[string]string{"approve": "approve", "deny": "deny", "revoke": "revoke"}

// decideAccess approves or denies a pending request, or revokes an active consent grant.
func (a *API) decideAccess(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.deciderCtx(w, r)
	if !ok {
		return
	}
	op, known := decisions[r.PathValue("decision")]
	g, err := a.db.GrantByID(r.Context(), r.PathValue("grantId"))
	now := time.Now()
	if !known || err != nil || g == nil || g.TenantID != t.ID || g.NoticeAt == nil || g.NoticeAt.After(now) {
		auth.Problem(w, http.StatusNotFound, "Request not found.")
		return
	}
	switch {
	case g.Requester == c.id.Sub:
		auth.Problem(w, http.StatusForbidden, "You can't decide your own request.")
		return
	case op != "revoke" && g.Kind == "breakglass":
		auth.Problem(w, http.StatusConflict, "Emergency access is decided by Shippin's approvers, not here.")
		return
	case op == "revoke" && g.Kind == "breakglass":
		auth.Problem(w, http.StatusConflict, "Emergency access can't be revoked here; it ends on its own at "+g.ExpiresAt.UTC().Format(time.RFC3339)+". Contact support with questions.")
		return
	case op != "revoke" && g.Status != "pending", op == "revoke" && !g.Active(now):
		auth.Problem(w, http.StatusConflict, "This request is already "+g.Effective(now)+".")
		return
	}
	who := displayName(c)
	e := grantEntry(r, g, c.id.Sub, who, op, "")
	if won, err := a.db.TransitionGrant(r.Context(), g.ID, op, c.id.Sub, who, e); err != nil || !won {
		auth.Problem(w, http.StatusConflict, "This request changed meanwhile; refresh and try again.")
		return
	}
	a.notifyDecided(g.ID, op, who)
	g, _ = a.db.GrantByID(r.Context(), g.ID)
	writeJSON(w, present([]db.Grant{*g})[0])
}

// tenantAudit is the customer's audit log (their tenant only).
func (a *API) tenantAudit(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.deciderCtx(w, r)
	if !ok {
		return
	}
	es, err := a.db.TenantAudit(r.Context(), t.ID, 300)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The audit log is unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": es})
}
