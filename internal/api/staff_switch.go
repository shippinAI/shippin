package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// Staff mode: the one-click tenant → provider panel switch (GitHub site-admin / WorkOS
// impersonation pattern). Opening a session needs a fresh IdP sign-in (prompt=login, max_age=0
// via /auth/login?reauth=1) and a reason; it lasts staff.session.minutes. Enter and exit are
// written in the same transaction as the session row. Routes still check permissions in gate().
type staffSwitchReq struct {
	Reason     string `json:"reason"`
	FromTenant string `json:"from_tenant"`
}

func (a *API) staffSwitchRoutes() []opRoute {
	return []opRoute{
		{"GET /v1/operator/staff-mode", permPanel, a.opStaffMode},
		{"POST /v1/operator/staff-mode", permPanel, a.opStaffEnter},
		{"POST /v1/operator/staff-mode/exit", permPanel, a.opStaffExit},
	}
}

func (a *API) staffModeView(s *db.StaffSession) map[string]any {
	out := map[string]any{"active": s != nil, "required": a.cfg.Staff.Session.Required,
		"minutes": int(a.cfg.Staff.Session.TTL().Minutes()), "operator_url": a.operatorURL()}
	if s != nil {
		out["session"] = s
	}
	return out
}

func (a *API) opStaffMode(w http.ResponseWriter, r *http.Request, c *caller) {
	s, err := a.db.ActiveStaffSession(r.Context(), c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Staff mode state is unavailable right now.")
		return
	}
	writeJSON(w, a.staffModeView(s))
}

// freshAuth reports whether the caller signed in within the re-auth window.
func freshAuth(at, now time.Time, window time.Duration) bool {
	return !at.IsZero() && now.Sub(at) <= window && !at.After(now.Add(time.Minute))
}

func (a *API) staffReauthWindow(c *caller) time.Duration {
	if c.installOwner || strings.HasPrefix(c.id.Sub, localSubPrefix) {
		return a.cfg.Staff.Session.LocalReauth()
	}
	return a.cfg.Staff.Session.Reauth()
}

func (a *API) opStaffEnter(w http.ResponseWriter, r *http.Request, c *caller) {
	var in staffSwitchReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Send a JSON body with a reason.")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 8 || n > 300 {
		auth.Problem(w, http.StatusUnprocessableEntity, "Give a reason of 8 to 300 characters; it goes in the audit log.")
		return
	}
	at := a.authTime(r)
	if !freshAuth(at, time.Now(), a.staffReauthWindow(c)) {
		auth.Problem(w, http.StatusPreconditionRequired, "Sign in again to enter staff mode.")
		return
	}
	e := teamEntry(r, c, "staff.mode_enter", in.Reason)
	s := db.StaffSession{Actor: c.id.Sub, Reason: in.Reason, AuthAt: at}
	if in.FromTenant != "" {
		if t, err := a.db.TenantByID(r.Context(), in.FromTenant); err == nil && t != nil {
			e.TenantID, e.TenantName, s.FromTenant = t.ID, t.Name, t.ID
		}
	}
	got, err := a.db.StartStaffSession(r.Context(), s, displayName(c), a.cfg.Staff.Session.TTL(), e)
	if err != nil {
		log.Printf("staff session start: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "Staff mode couldn't be recorded, so it wasn't started.")
		return
	}
	writeJSON(w, a.staffModeView(got))
}

func (a *API) opStaffExit(w http.ResponseWriter, r *http.Request, c *caller) {
	s, _ := a.db.ActiveStaffSession(r.Context(), c.id.Sub)
	e := teamEntry(r, c, "staff.mode_exit", "")
	back := "https://" + a.cfg.ConsoleHost
	if s != nil && s.FromTenant != "" {
		e.TenantID, back = s.FromTenant, back+"/t/"+s.FromTenant
	}
	if _, err := a.db.EndStaffSession(r.Context(), c.id.Sub, e); err != nil {
		log.Printf("staff session end: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "Staff mode couldn't be ended right now.")
		return
	}
	writeJSON(w, map[string]any{"active": false, "console_url": back})
}

// staffSessionGate refuses permissioned operator routes without an open session when
// staff.session.required is on. Panel-level routes (access, staff mode itself) stay open.
func (a *API) staffSessionGate(w http.ResponseWriter, r *http.Request, c *caller, perm string) bool {
	if !a.cfg.Staff.Session.Required || perm == permPanel {
		return true
	}
	s, err := a.db.ActiveStaffSession(r.Context(), c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Staff mode state is unavailable right now.")
		return false
	}
	if s == nil {
		auth.Problem(w, http.StatusForbidden, "Enter staff mode first (fresh sign-in and a reason).")
		return false
	}
	return true
}
