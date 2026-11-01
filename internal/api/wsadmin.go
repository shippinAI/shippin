package api

import (
	"log"
	"net/http"
	"slices"
	"time"
)

// adminHost: a platform tool host (operator.admin_hosts) served behind forward_auth.
func (a *API) adminHost(host string) bool {
	return slices.Contains(a.cfg.Operator.AdminHosts, host)
}

// adminAllowed: only the platform admin role opens admin hosts. Opens and refusals are logged
// once per log window per user and host.
func (a *API) adminAllowed(r *http.Request, tok, host string) bool {
	c, err := a.callerFor(r, tok)
	if err != nil {
		return false
	}
	return a.adminPermit(r, c, host)
}

// adminPermit decides for a resolved caller.
func (a *API) adminPermit(r *http.Request, c *caller, host string) bool {
	ok := a.adminHost(host) && a.can(r, c, "host.admin_tools")
	window, _ := time.ParseDuration(a.cfg.Access.LogWindow)
	if a.seen.due(c.id.Sub+"|admin|"+host, window, time.Now()) {
		log.Printf("admin host %s: user=%s role=%q founder=%v allowed=%v", host, c.id.Sub, c.op, c.founder, ok)
	}
	return ok
}

// founderOpen logs a founder opening a workspace, launcher or utility host (once per window).
func (a *API) founderOpen(r *http.Request, c *caller, host string) bool {
	window, _ := time.ParseDuration(a.cfg.Access.LogWindow)
	if a.seen.due(c.id.Sub+"|founder|"+host, window, time.Now()) {
		log.Printf("founder host %s: user=%s allowed=true", host, c.id.Sub)
	}
	a.auditOnce(r, c, "authz.founder", "host.open", host)
	return true
}
