package api

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
)

// wsAllowed: the workspace serving host exists and the token's user may open it: a member who
// manages the tenant or created the workspace, or a platform admin with an active grant
// covering it. A grant-based open is audited (deduped per window) and refused if it can't be.
// An AI activation terminal opens for its owner only (utilityAllowed).
func (a *API) wsAllowed(r *http.Request, tok, host string) bool {
	if a.sparkHost(host) {
		return a.sparkAllowed(r, tok, host)
	}
	if a.adminHost(host) {
		return a.adminAllowed(r, tok, host)
	}
	if a.utilityHost(host) {
		return a.utilityAllowed(r, tok, host)
	}
	ws, err := a.db.WorkspaceByHost(r.Context(), workspaceHost(host))
	if err != nil || ws == nil {
		return false
	}
	c, err := a.callerFor(r, tok)
	if err != nil {
		return false
	}
	if c.founder {
		return a.founderOpen(r, c, host)
	}
	for i := range c.tenants {
		t := &c.tenants[i]
		if t.ID != ws.TenantID {
			continue
		}
		if t.member() {
			return c.canManage(t) || ws.CreatedBy == c.id.Sub
		}
		return c.canManageWS(t, ws.ID) && a.auditOpen(r, c, t, ws.ID, host)
	}
	return false
}

// auditOpen logs a grant holder opening the IDE or a preview, once per window per host.
func (a *API) auditOpen(r *http.Request, c *caller, t *tenantView, wsID, host string) bool {
	g := t.grantFor(wsID)
	window, _ := time.ParseDuration(a.cfg.Access.LogWindow)
	key := c.id.Sub + "|" + host + "|" + g.ID
	if !a.seen.due(key, window, time.Now()) {
		return true
	}
	kind := "ide"
	if previewLabel.MatchString(host) {
		kind = "preview"
	}
	if !a.auditUse(r, c, t, wsID, kind, host) {
		a.seen.forget(key)
		return false
	}
	return true
}

// wsCheck is Caddy's forward_auth target. 2xx = proxy to the IDE; otherwise the redirect is returned.
func (a *API) wsCheck(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.Split(r.Host, ":")[0])
	if c, err := r.Cookie(auth.WSCookie); err == nil {
		if s := a.flow.Store.Get(r.Context(), c.Value); s.Workspace(host) && a.wsAllowed(r, s.AccessToken, host) {
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	rd := "https://" + host + r.Header.Get("X-Forwarded-Uri")
	http.Redirect(w, r, "https://"+a.cfg.ConsoleHost+"/auth/ws-login?rd="+url.QueryEscape(rd), http.StatusFound)
}

// wsLogin runs on the console host: needs a console session, then hands a one-time code to the workspace host.
func (a *API) wsLogin(w http.ResponseWriter, r *http.Request) {
	rd, err := url.Parse(r.URL.Query().Get("rd"))
	if err != nil || rd.Scheme != "https" || rd.Host == "" {
		auth.Problem(w, http.StatusBadRequest, "Invalid return address.")
		return
	}
	tok := a.flow.SessionToken(r)
	if tok == "" {
		http.Redirect(w, r, "/auth/login?rd="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	host := strings.ToLower(rd.Hostname())
	if !a.wsAllowed(r, tok, host) {
		auth.Problem(w, http.StatusForbidden, "You don't have access to this workspace.")
		return
	}
	code, err := a.flow.Store.New(r.Context(), &auth.Session{AccessToken: tok, ReturnTo: host, State: "code", Expires: time.Now().Add(time.Minute)})
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	path := rd.RequestURI()
	http.Redirect(w, r, "https://"+host+"/_shippin/auth?code="+url.QueryEscape(code)+"&rd="+url.QueryEscape(path), http.StatusFound)
}

// wsCode runs on the workspace host (Caddy routes /_shippin/* here): swaps the code for a host cookie.
func (a *API) wsCode(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.Split(r.Host, ":")[0])
	code := r.URL.Query().Get("code")
	s := a.flow.Store.Get(r.Context(), code)
	a.flow.Store.Delete(r.Context(), code)
	if s == nil || s.State != "code" || s.ReturnTo != host {
		auth.Problem(w, http.StatusForbidden, "This sign-in link expired. Open the workspace again from the console.")
		return
	}
	exp := time.Now().Add(12 * time.Hour)
	id, err := a.flow.Store.New(r.Context(), &auth.Session{AccessToken: s.AccessToken, ReturnTo: host, Expires: exp})
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	auth.SetWSCookie(w, id, exp)
	http.Redirect(w, r, localPath(r.URL.Query().Get("rd")), http.StatusFound)
}

var previewLabel = regexp.MustCompile(`^[0-9]{4,5}--`)

// workspaceHost maps a preview host <port>--<label>.<base> to the workspace host <label>.<base>.
func workspaceHost(host string) string { return previewLabel.ReplaceAllString(host, "") }
