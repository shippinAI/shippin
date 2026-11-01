package api

import (
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// breakGlassPath decides how a break-glass request proceeds: "direct" (no approval required,
// or legacy mode), "approval" (another approver exists: wait for them) or "sole_owner" (no
// other approver; the requester is an Owner and the setting allows it: active now, flagged).
// msg is set when it can't proceed at all. Self-approval never happens.
func (a *API) breakGlassPath(r *http.Request, c *caller) (path, msg string) {
	s := a.teamSettings(r.Context())
	if a.authzMode(r.Context()) == "legacy" || !s.BreakGlassApproval {
		return "direct", ""
	}
	roles, err := a.store().StaffRoles(r.Context())
	if err != nil {
		return "", "Approvers couldn't be checked, so break-glass wasn't started."
	}
	hs, err := a.staffHolders(r.Context(), roles, nil)
	if err != nil {
		log.Printf("break-glass approvers: %v", err)
		return "", "Approvers couldn't be checked, so break-glass wasn't started."
	}
	if a.countWith(r.Context(), hs, "access.approve_breakglass", c.id.Sub, true) > 0 {
		return "approval", ""
	}
	if owner, _ := a.decide(r.Context(), c, permOwner); owner && s.SoleOwnerActivation {
		return "sole_owner", ""
	}
	return "", "No one else can approve break-glass right now, and sole-Owner activation is off or you aren't an Owner."
}

// opBreakGlassPending lists break-glass requests waiting for an approver.
func (a *API) opBreakGlassPending(w http.ResponseWriter, r *http.Request, c *caller) {
	gs, err := a.db.AllGrants(r.Context(), 300)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Access grants are unavailable right now.")
		return
	}
	lapse := time.Duration(a.teamSettings(r.Context()).ApprovalMinutes) * time.Minute
	out := []db.Grant{}
	for _, g := range gs {
		if g.Kind == "breakglass" && g.Status == "pending" && time.Since(g.CreatedAt) <= lapse {
			out = append(out, g)
		}
	}
	writeJSON(w, map[string]any{"items": present(out), "self": c.id.Sub, "approval_minutes": lapse.Minutes()})
}

// opDecideBreakGlass approves or denies someone else's pending break-glass, with a fresh
// sign-in. The requester can never decide their own; lapsed requests are closed instead.
func (a *API) opDecideBreakGlass(w http.ResponseWriter, r *http.Request, c *caller) {
	op := map[string]string{"approve": "approve", "deny": "deny"}[r.PathValue("decision")]
	g, err := a.db.GrantByID(r.Context(), r.PathValue("grantId"))
	if op == "" || err != nil || g == nil || g.Kind != "breakglass" {
		auth.Problem(w, http.StatusNotFound, "Break-glass request not found.")
		return
	}
	if g.Requester == c.id.Sub {
		a.auditOnce(r, c, "authz.denied", "access.approve_breakglass", "self-approval refused")
		auth.Problem(w, http.StatusForbidden, "You can't decide your own break-glass request.")
		return
	}
	if g.Status != "pending" {
		auth.Problem(w, http.StatusConflict, "This request is already "+g.Effective(time.Now())+".")
		return
	}
	at := a.authTime(r)
	if at.IsZero() || time.Since(at) > time.Duration(a.cfg.Access.BreakGlass.ReauthMinutes)*time.Minute {
		auth.Problem(w, http.StatusPreconditionRequired, "Sign in again to confirm it's you before deciding break-glass.")
		return
	}
	who := displayName(c)
	if time.Since(g.CreatedAt) > time.Duration(a.teamSettings(r.Context()).ApprovalMinutes)*time.Minute {
		_, _ = a.db.TransitionGrant(r.Context(), g.ID, "cancel", c.id.Sub, who, grantEntry(r, g, c.id.Sub, who, "lapsed", "No decision within the approval window"))
		auth.Problem(w, http.StatusConflict, "This request lapsed; the requester has to ask again.")
		return
	}
	if won, err := a.db.TransitionGrant(r.Context(), g.ID, op, c.id.Sub, who, grantEntry(r, g, c.id.Sub, who, op, "")); err != nil || !won {
		auth.Problem(w, http.StatusConflict, "This request changed meanwhile; refresh and try again.")
		return
	}
	if op == "approve" {
		log.Printf("BREAK-GLASS %s approved by %s for %s", g.ID, c.id.Sub, g.Requester)
		a.notifyBreakGlass(g.ID)
	}
	g, _ = a.db.GrantByID(r.Context(), g.ID)
	writeJSON(w, present([]db.Grant{*g})[0])
}
