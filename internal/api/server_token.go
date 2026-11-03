package api

import (
	"errors"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/servers"
)

// serverProblem maps registry errors to problems (no secret or hash is ever in the message).
func serverProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servers.ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "Server not found.")
	case errors.Is(err, servers.ErrReadOnly):
		auth.Problem(w, http.StatusForbidden, "This site is planned by the platform; it can't be enrolled or revoked here.")
	case errors.Is(err, servers.ErrState):
		auth.Problem(w, http.StatusConflict, "That isn't possible in the server's current state.")
	default:
		auth.Problem(w, http.StatusServiceUnavailable, "Servers are unavailable right now.")
	}
}

// issueServerToken returns a single-use token ONCE. It is deliberately not wrapped in idempotent():
// a replay cache would store the token in clear. A lost response means: issue another.
func (a *API) issueServerToken(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	id := r.PathValue("serverId")
	e := db.AuditEntry{TenantID: t.ID, Actor: c.id.Sub, ActorName: displayName(c), Action: "server.enroll_token",
		Detail: id, IP: clientIP(r)}
	if !a.auditFirst(w, r, e) {
		return
	}
	ttl := time.Duration(a.cfg.Servers.TokenTTLSeconds) * time.Second
	iss, err := a.serverStore().IssueToken(r.Context(), t.ID, id, c.id.Sub, ttl)
	if err != nil {
		serverProblem(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, iss)
}

func (a *API) revokeServer(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	id := r.PathValue("serverId")
	e := db.AuditEntry{TenantID: t.ID, Actor: c.id.Sub, ActorName: displayName(c), Action: "server.revoke", Detail: id, IP: clientIP(r)}
	if !a.auditFirst(w, r, e) {
		return
	}
	if err := a.serverStore().Revoke(r.Context(), t.ID, id); err != nil {
		serverProblem(w, err)
		return
	}
	v, err := a.serverStore().Get(r.Context(), t.ID, id)
	if err != nil {
		serverProblem(w, err)
		return
	}
	writeJSON(w, v)
}
