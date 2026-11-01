package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/db"
)

// teamSettings is the stored team settings (cached briefly); on a read error the last good
// copy, else the defaults, apply.
func (a *API) teamSettings(ctx context.Context) db.StaffSettings {
	s := &a.staffCache
	s.mu.Lock()
	if s.settings != nil && time.Since(s.setAt) < 5*time.Second {
		v := *s.settings
		s.mu.Unlock()
		return v
	}
	last := s.settings
	s.mu.Unlock()
	v, err := a.store().StaffSettings(ctx)
	if err != nil {
		log.Printf("staff: settings: %v", err)
		if last != nil {
			return *last
		}
		return db.DefaultStaffSettings
	}
	s.mu.Lock()
	s.settings, s.setAt = &v, time.Now()
	s.mu.Unlock()
	return v
}

// authzMode is the config override (rollback switch) or the stored mode.
func (a *API) authzMode(ctx context.Context) string {
	if m := a.cfg.Staff.Mode; m != "" {
		return m
	}
	if m := a.teamSettings(ctx).Mode; m != "" {
		return m
	}
	return "legacy"
}

// staffCan is the new decision: deny by default, permission from the caller's roles.
func (a *API) staffCan(ctx context.Context, c *caller, perm string) bool {
	v := a.staffOf(ctx, c)
	if perm == permPanel {
		return len(v.Perms) > 0
	}
	return v.Perms[perm]
}

// decide is one permission for the caller according to the mode (no side effects):
//   - legacy: the old single gate (IAM platform admin) only;
//   - shadow: the old gate decides; note reports a mismatch with the new decision;
//   - enforce: the new decision; IAM platform admins keep access only through the legacy
//     fallback (note "legacy") when that setting is on.
func (a *API) decide(ctx context.Context, c *caller, perm string) (ok bool, note string) {
	if c.founder {
		return true, "founder"
	}
	old := a.isPlatformAdmin(c)
	switch a.authzMode(ctx) {
	case "shadow":
		if nv := a.staffCan(ctx, c, perm); nv != old {
			return old, map[bool]string{true: "new allows, legacy denies", false: "new denies, legacy allows"}[nv]
		}
		return old, ""
	case "enforce":
		if a.staffCan(ctx, c, perm) {
			return true, ""
		}
		if old && a.teamSettings(ctx).LegacyFallback {
			return true, "legacy"
		}
		return false, ""
	}
	return old, ""
}

// can is decide plus its audit trail (shadow mismatches, legacy fallback use).
func (a *API) can(r *http.Request, c *caller, perm string) bool {
	ok, note := a.decide(r.Context(), c, perm)
	switch {
	case note == "founder":
		a.auditOnce(r, c, "authz.founder", perm, "allowed by the Nomen founder role")
	case note == "legacy":
		a.auditOnce(r, c, "authz.legacy_iam_owner", perm, "allowed through the IAM owner fallback")
	case note != "":
		a.auditOnce(r, c, "authz.shadow_mismatch", perm, note)
	}
	return ok
}

// auditOnce appends an authorization entry, at most once per user, action and permission per
// log window, so a polling page can't flood the log.
func (a *API) auditOnce(r *http.Request, c *caller, action, perm, detail string) {
	window, err := time.ParseDuration(a.cfg.Access.LogWindow)
	if err != nil || window <= 0 {
		window = 10 * time.Minute
	}
	if !a.seen.due(c.id.Sub+"|"+action+"|"+perm, window, time.Now()) {
		return
	}
	e := db.AuditEntry{Actor: c.id.Sub, ActorName: displayName(c), Action: action,
		Detail: perm + " · " + r.Method + " " + r.URL.Path + " · " + detail, IP: clientIP(r)}
	if err := a.store().AddAudit(r.Context(), e); err != nil {
		log.Printf("audit %s for %s: %v", action, c.id.Sub, err)
	}
}

// touchStaff records console activity for staff (at most every 5 minutes per user).
func (a *API) touchStaff(r *http.Request, c *caller) {
	if !a.seen.due(c.id.Sub+"|seen", 5*time.Minute, time.Now()) {
		return
	}
	if err := a.store().TouchStaff(r.Context(), c.id.Sub); err != nil {
		log.Printf("staff seen %s: %v", c.id.Sub, err)
	}
}
