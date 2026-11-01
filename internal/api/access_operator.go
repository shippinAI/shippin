package api

import (
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// opRequestAccess asks a tenant's owners/admins for time-boxed access (pending until they decide).
func (a *API) opRequestAccess(w http.ResponseWriter, r *http.Request, c *caller) {
	in, ok := decodeAccess(w, r)
	if !ok {
		return
	}
	dur, msg := checkRequest(a.cfg.Access, in)
	if msg != "" {
		auth.Problem(w, http.StatusBadRequest, msg)
		return
	}
	t, scope, ok := a.accessTarget(w, r, c, in)
	if !ok {
		return
	}
	if open, err := a.db.OpenGrant(r.Context(), t.ID, in.WorkspaceID, c.id.Sub); err != nil {
		log.Printf("access request open check: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "Access requests are unavailable right now.")
		return
	} else if open {
		auth.Problem(w, http.StatusConflict, "You already have a pending or active grant for this scope.")
		return
	}
	id, _ := uuid.NewV7()
	now := time.Now()
	g := db.Grant{ID: id.String(), TenantID: t.ID, WorkspaceID: in.WorkspaceID, Kind: "request", Requester: c.id.Sub,
		RequesterName: displayName(c), Reason: in.Reason, DurationS: int(dur.Seconds()), Status: "pending", NoticeAt: &now}
	e := grantEntry(r, &g, c.id.Sub, g.RequesterName, "request", scope+" for "+in.Duration+": "+in.Reason)
	if err := a.db.CreateGrant(r.Context(), g, e); err != nil {
		log.Printf("access request for tenant %s: %v", t.ID, err)
		auth.Problem(w, http.StatusServiceUnavailable, "The request couldn't be saved.")
		return
	}
	a.notifyRequested(g.ID)
	writeStatus(w, http.StatusCreated, map[string]string{"id": g.ID, "status": g.Status})
}

// opBreakGlass grants short access without customer approval: access.breakglass (gate),
// mandatory category and justification, fresh IdP sign-in, audited in one transaction. When
// the team settings require an approver it is stored pending (see breakGlassPath).
func (a *API) opBreakGlass(w http.ResponseWriter, r *http.Request, c *caller) {
	in, ok := decodeAccess(w, r)
	if !ok {
		return
	}
	now := time.Now()
	cat, msg, reauth := checkBreakGlass(a.cfg.Access, in, a.authTime(r), now)
	if reauth {
		auth.Problem(w, http.StatusPreconditionRequired, msg)
		return
	}
	if msg != "" {
		auth.Problem(w, http.StatusBadRequest, msg)
		return
	}
	t, scope, ok := a.accessTarget(w, r, c, in)
	if !ok {
		return
	}
	if in.Withhold && !a.can(r, c, "access.legal_hold") {
		auth.Problem(w, http.StatusForbidden, "Delaying the customer notice needs the access.legal_hold permission.")
		return
	}
	path, msg := a.breakGlassPath(r, c)
	if msg != "" {
		auth.Problem(w, http.StatusConflict, msg)
		return
	}
	dur, _ := time.ParseDuration(a.cfg.Access.BreakGlass.Duration)
	notice := noticeAt(cat, in.Withhold, now)
	id, _ := uuid.NewV7()
	g := db.Grant{ID: id.String(), TenantID: t.ID, WorkspaceID: in.WorkspaceID, Kind: "breakglass", Requester: c.id.Sub,
		RequesterName: displayName(c), Reason: in.Justification, Category: cat.ID, DurationS: int(dur.Seconds()),
		Status: "approved", DecidedBy: c.id.Sub, DecidedName: displayName(c), NoticeAt: &notice}
	what := cat.Label + " — " + scope + " for " + a.cfg.Access.BreakGlass.Duration + ": " + in.Justification
	es := []db.AuditEntry{grantEntry(r, &g, c.id.Sub, g.RequesterName, "activate", what)}
	if path == "approval" {
		g.Status, g.DecidedBy, g.DecidedName = "pending", "", ""
		es = []db.AuditEntry{grantEntry(r, &g, c.id.Sub, g.RequesterName, "request", what+" (waiting for an approver)")}
	} else if path == "sole_owner" {
		es = append(es, grantEntry(r, &g, c.id.Sub, g.RequesterName, "unapproved",
			"Activated by the only Owner without a second approver; flagged for review"))
	}
	if notice.After(now) {
		es = append(es, grantEntry(r, &g, c.id.Sub, g.RequesterName, "notice_withheld",
			"Customer notice delayed until "+notice.UTC().Format(time.RFC3339)+" ("+cat.Label+")"))
	}
	if err := a.db.CreateGrant(r.Context(), g, es...); err != nil {
		log.Printf("break-glass for tenant %s: %v", t.ID, err)
		auth.Problem(w, http.StatusServiceUnavailable, "Break-glass couldn't be recorded, so it wasn't activated.")
		return
	}
	if g.Status == "pending" {
		log.Printf("BREAK-GLASS requested by %s on tenant %s (%s, %s), waiting for approval", c.id.Sub, t.ID, cat.ID, scope)
		writeStatus(w, http.StatusAccepted, map[string]any{"id": g.ID, "status": g.Status, "notice_at": notice})
		return
	}
	log.Printf("BREAK-GLASS activated by %s on tenant %s (%s, %s, %s)", c.id.Sub, t.ID, cat.ID, scope, path)
	a.notifyBreakGlass(g.ID)
	writeStatus(w, http.StatusCreated, map[string]any{"id": g.ID, "status": g.Status, "notice_at": notice})
}

// opEndGrant lets the requesting admin cancel a pending request or end an active grant early.
func (a *API) opEndGrant(w http.ResponseWriter, r *http.Request, c *caller) {
	g, err := a.db.GrantByID(r.Context(), r.PathValue("grantId"))
	if err != nil || g == nil || g.Requester != c.id.Sub {
		auth.Problem(w, http.StatusNotFound, "Grant not found.")
		return
	}
	op := map[string]string{"pending": "cancel", "approved": "end"}[g.Effective(time.Now())]
	if op == "" {
		auth.Problem(w, http.StatusConflict, "This grant is already closed.")
		return
	}
	won, err := a.db.TransitionGrant(r.Context(), g.ID, op, c.id.Sub, displayName(c), grantEntry(r, g, c.id.Sub, displayName(c), op, ""))
	if err != nil || !won {
		auth.Problem(w, http.StatusConflict, "This grant changed meanwhile; refresh and try again.")
		return
	}
	writeJSON(w, map[string]string{"id": g.ID, "status": map[string]string{"cancel": "cancelled", "end": "ended"}[op]})
}
