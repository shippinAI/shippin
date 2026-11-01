package api

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"shippin.cloud/console/internal/auth"
)

// The devspace page frames the IDE (workspace host), but the console and the IdP send
// X-Frame-Options: DENY, so a sign-in hop must never run inside the frame. Instead the page
// asks the workspace host whether its cookie is valid (/_shippin/status, credentialed CORS from
// the console origin only) and, if not, signs in through a pop-up: /_shippin/popup → console
// /auth/ws-login → /_shippin/auth (cookie) → /_shippin/done, which tells the opener and closes.

func (a *API) consoleOrigin() string { return "https://" + a.cfg.ConsoleHost }

func requestHost(r *http.Request) string { return strings.ToLower(strings.Split(r.Host, ":")[0]) }

// wsStatus: 204 when the workspace cookie is valid and still allowed, else 401. No body.
func (a *API) wsStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", a.consoleOrigin())
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Cache-Control", "no-store")
	host := requestHost(r)
	if c, err := r.Cookie(auth.WSCookie); err == nil {
		if s := a.flow.Store.Get(r.Context(), c.Value); s.Workspace(host) && a.wsAllowed(r, s.AccessToken, host) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	w.WriteHeader(http.StatusUnauthorized)
}

// wsPopup starts the pop-up sign-in for this workspace host.
func (a *API) wsPopup(w http.ResponseWriter, r *http.Request) {
	rd := "https://" + requestHost(r) + "/_shippin/done"
	http.Redirect(w, r, a.consoleOrigin()+"/auth/ws-login?rd="+url.QueryEscape(rd), http.StatusFound)
}

var doneTmpl = template.Must(template.New("done").Parse(`<!doctype html><meta charset="utf-8">
<title>Signed in</title><p>Signed in. You can close this window.</p>
<script>if (window.opener) { window.opener.postMessage({type: "shippin-ws-ready", host: {{.Host}}}, {{.Origin}}); }
window.close();</script>`))

// wsDone ends the pop-up: it only signals the console origin; the page re-checks /_shippin/status.
func (a *API) wsDone(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = doneTmpl.Execute(w, map[string]string{"Host": requestHost(r), "Origin": a.consoleOrigin()})
}
