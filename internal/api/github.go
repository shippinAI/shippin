package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
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

// newGitHub builds the App client from config; nil (feature off) when disabled or misconfigured.
func newGitHub(g config.GitHub) *gh.App {
	if !g.Enabled {
		return nil
	}
	app, err := gh.New(g.ClientID, g.ClientSecret, g.PrivateKey, g.APIBase, g.WebBase)
	if err != nil {
		log.Printf("github: disabled: %v", err)
		return nil
	}
	return app
}

// stateFor signs tenant|account|nonce|expiry so the setup redirect can't be replayed or retargeted.
func (a *API) stateFor(tenant, account string, now time.Time) string {
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	exp := now.Add(time.Duration(a.cfg.GitHub.StateMinutes) * time.Minute).Unix()
	body := fmt.Sprintf("%s.%s.%s.%d", tenant, account, base64.RawURLEncoding.EncodeToString(nonce), exp)
	return body + "." + a.stateMAC(body)
}

func (a *API) stateMAC(body string) string {
	k := hmac.New(sha256.New, []byte(a.cfg.SessionKey))
	k.Write([]byte("github-install-state"))
	m := hmac.New(sha256.New, k.Sum(nil))
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// checkState returns the tenant a valid, unexpired state was issued for to account.
func (a *API) checkState(state, account string, now time.Time) (string, error) {
	i := strings.LastIndex(state, ".")
	if i < 0 || a.cfg.SessionKey == "" || !hmac.Equal([]byte(state[i+1:]), []byte(a.stateMAC(state[:i]))) {
		return "", errors.New("bad state")
	}
	f := strings.Split(state[:i], ".")
	exp, err := strconv.ParseInt(f[len(f)-1], 10, 64)
	if len(f) != 4 || err != nil || now.Unix() > exp || f[1] != account {
		return "", errors.New("expired or foreign state")
	}
	return f[0], nil
}

// githubInstall sends an org owner/admin to GitHub to install the App for this tenant.
func (a *API) githubInstall(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok || !a.githubFor(w, t) {
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
	u := fmt.Sprintf("%s/apps/%s/installations/new?state=%s", a.cfg.GitHub.WebBase, a.cfg.GitHub.AppSlug,
		url.QueryEscape(a.stateFor(t.ID, acct, time.Now())))
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
	tenant, serr := a.checkState(q.Get("state"), acct, time.Now())
	inst, ierr := strconv.ParseInt(q.Get("installation_id"), 10, 64)
	if err != nil || serr != nil || ierr != nil || inst <= 0 || q.Get("code") == "" || a.gh == nil {
		auth.Problem(w, http.StatusBadRequest, "This GitHub link expired or is invalid. Start again from Shippin.")
		return
	}
	t := c.grantedTenant(tenant)
	if t == nil || !c.canDecide(t) || !a.cfg.GitHub.Allows(t.ID) {
		auth.Problem(w, http.StatusForbidden, "Connecting GitHub needs the org owner or admin role.")
		return
	}
	in, err := a.gh.VerifyInstall(r.Context(), q.Get("code"), inst)
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
	http.Redirect(w, r, "/t/"+t.ID+"/projects?source=github", http.StatusFound)
}

// githubFor fails closed when the feature is off for t.
func (a *API) githubFor(w http.ResponseWriter, t *tenantView) bool {
	if a.gh == nil || !a.cfg.GitHub.Allows(t.ID) || !t.member() {
		auth.Problem(w, http.StatusNotFound, "GitHub import isn't available in this tenant.")
		return false
	}
	return true
}
