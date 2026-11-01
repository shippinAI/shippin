package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// clientIP is the browser's address: the first X-Forwarded-For hop set by the edge (the
// console only listens behind it), else the socket peer.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// kindPrefix keeps break-glass entries distinct ("breakglass.*" vs "access.*").
func kindPrefix(g *db.Grant) string {
	if g.Kind == "breakglass" {
		return "breakglass."
	}
	return "access."
}

func grantEntry(r *http.Request, g *db.Grant, actor, name, action, detail string) db.AuditEntry {
	return db.AuditEntry{TenantID: g.TenantID, WorkspaceID: g.WorkspaceID, GrantID: g.ID, Actor: actor,
		ActorName: name, Action: kindPrefix(g) + action, Detail: detail, IP: clientIP(r), BreakGlass: g.Kind == "breakglass"}
}

// auditUse records a platform admin acting on a tenant through a grant. Members are not
// logged here. It fails closed: when the entry can't be written the caller must refuse.
func (a *API) auditUse(r *http.Request, c *caller, t *tenantView, wsID, action, detail string) bool {
	if t.member() {
		return true
	}
	g := t.grantFor(wsID)
	if g == nil {
		return false
	}
	e := grantEntry(r, g, c.id.Sub, c.id.Name, action, detail)
	e.WorkspaceID = wsID
	return a.db.AddAudit(r.Context(), e) == nil
}

// refuseUnaudited writes the 503 for a use that couldn't be audited.
func refuseUnaudited(w http.ResponseWriter) {
	auth.Problem(w, http.StatusServiceUnavailable, "Access couldn't be recorded in the audit log, so it was refused.")
}

// seenCache dedupes per-request access entries (forward_auth runs on every request) to one
// per admin, host and grant per window; the first request after the window logs again.
type seenCache struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (s *seenCache) due(key string, window time.Duration, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		s.last = map[string]time.Time{}
	}
	if t, ok := s.last[key]; ok && now.Sub(t) < window {
		return false
	}
	for k, t := range s.last {
		if now.Sub(t) >= window {
			delete(s.last, k)
		}
	}
	s.last[key] = now
	return true
}

func (s *seenCache) forget(key string) {
	s.mu.Lock()
	delete(s.last, key)
	s.mu.Unlock()
}
