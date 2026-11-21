package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/gh"
)

// githubRoutes (M3): install the Shippin GitHub App on a tenant, list its repos, import one.
func (a *API) githubRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/github/install", a.flow.Require(a.githubInstall))
	mux.HandleFunc("GET /v1/github/setup", a.flow.Require(a.githubSetup))
	mux.HandleFunc("GET /v1/github/callback", a.flow.Require(a.githubSetup))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/github/repos", a.flow.Require(a.githubRepos))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/github/import", a.flow.Require(a.githubImport))
	mux.HandleFunc("POST /v1/github/webhook", a.githubWebhook)
}

// installReturns are the panel pages the install flow may send someone back to, by name. Only a
// name travels, inside the signed state, so the redirect can't be pointed anywhere else.
var installReturns = map[string]string{"projects": "projects?source=github", "runners": "runners/repositories"}

// stateFor signs tenant|account|nonce|expiry|return so the setup redirect can't be replayed or
// retargeted. An unknown return is the projects page.
func (a *API) stateFor(ctx context.Context, tenant, account, ret string, now time.Time) string {
	if _, ok := installReturns[ret]; !ok {
		ret = "projects"
	}
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	_, g := a.github(ctx)
	exp := now.Add(time.Duration(g.StateMinutes) * time.Minute).Unix()
	body := fmt.Sprintf("%s.%s.%s.%d.%s", tenant, account, base64.RawURLEncoding.EncodeToString(nonce), exp, ret)
	return body + "." + a.stateMAC(body)
}

func (a *API) stateMAC(body string) string {
	k := hmac.New(sha256.New, []byte(a.cfg.SessionKey))
	k.Write([]byte("github-install-state"))
	m := hmac.New(sha256.New, k.Sum(nil))
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// checkState returns the tenant a valid, unexpired state was issued for to account, and the page to
// return to.
func (a *API) checkState(state, account string, now time.Time) (tenant, ret string, err error) {
	i := strings.LastIndex(state, ".")
	if i < 0 || a.cfg.SessionKey == "" || !hmac.Equal([]byte(state[i+1:]), []byte(a.stateMAC(state[:i]))) {
		return "", "", errors.New("bad state")
	}
	f := strings.Split(state[:i], ".")
	if len(f) != 5 || installReturns[f[4]] == "" {
		return "", "", errors.New("bad state")
	}
	exp, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil || now.Unix() > exp || f[1] != account {
		return "", "", errors.New("expired or foreign state")
	}
	return f[0], f[4], nil
}

// githubInstall sends an org owner/admin to GitHub to install the App for this tenant. ?return=runners
// brings them back to the build runners page afterwards; the default is the projects page.
func (a *API) githubInstall(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	_, g, ok := a.githubFor(w, r, t)
	if !ok {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Connecting GitHub needs the org owner or admin role.")
		return
	}
	acct, err := a.db.UpsertAccount(r.Context(), c.id.Sub, c.id.Email)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "GitHub is unavailable right now.")
		return
	}
	u := fmt.Sprintf("%s/apps/%s/installations/new?state=%s", g.WebBase, g.AppSlug,
		url.QueryEscape(a.stateFor(r.Context(), t.ID, acct, r.URL.Query().Get("return"), time.Now())))
	http.Redirect(w, r, u, http.StatusFound)
}

// githubSetup is GitHub's redirect after install: the signed state names the tenant, and the
// installation must be visible to the signing-in GitHub user (verified with their one-time token).
func (a *API) githubSetup(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	acct, err := a.db.UpsertAccount(r.Context(), c.id.Sub, c.id.Email)
	tenant, ret, serr := a.checkState(q.Get("state"), acct, time.Now())
	inst, ierr := strconv.ParseInt(q.Get("installation_id"), 10, 64)
	app, g := a.github(r.Context())
	if err != nil || serr != nil || ierr != nil || inst <= 0 || q.Get("code") == "" || app == nil {
		auth.Problem(w, http.StatusBadRequest, "This GitHub link expired or is invalid. Start again from Shippin.")
		return
	}
	t := c.grantedTenant(tenant)
	if t == nil || !c.canDecide(t) || !g.Allows(t.ID) {
		auth.Problem(w, http.StatusForbidden, "Connecting GitHub needs the org owner or admin role.")
		return
	}
	in, err := app.VerifyInstall(r.Context(), q.Get("code"), inst)
	if err != nil {
		auth.Problem(w, http.StatusForbidden, "That GitHub installation isn't yours.")
		return
	}
	rec := db.GitHubInstallation{ID: inst, TenantID: t.ID, AccountLogin: in.Account.Login, AccountType: in.Account.Type,
		RepoSelection: in.RepoSelection}
	if err := a.db.BindInstallation(r.Context(), rec, acct); err != nil {
		auth.Problem(w, http.StatusConflict, "That GitHub installation is already connected to another Shippin tenant.")
		return
	}
	http.Redirect(w, r, "/t/"+t.ID+"/"+installReturns[ret], http.StatusFound)
}

// githubFor is the App and its section for t, or answers 404 (ok false) when the feature is off for t.
func (a *API) githubFor(w http.ResponseWriter, r *http.Request, t *tenantView) (*gh.App, config.GitHub, bool) {
	app, g := a.github(r.Context())
	if app == nil || !g.Allows(t.ID) || !t.member() {
		auth.Problem(w, http.StatusNotFound, "GitHub import isn't available in this tenant.")
		return nil, g, false
	}
	return app, g, true
}
