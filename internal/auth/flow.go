package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"shippin.cloud/console/internal/config"
)

const (
	cookieName = "shippin_session"
	flowCookie = "shippin_login"
)

// Flow wires the PKCE authorization-code login against Nomen.
type Flow struct {
	cfg    *config.Config
	oa     oauth2.Config
	ops    oauth2.Config // the operator app (ops origin); ClientID "" = not registered
	Client *http.Client
	Store  *Store
	cache  *idCache
	// Keys resolves a person's access key to their directory user and the sign-in method it carries.
	Keys func(ctx context.Context, token string) (userID, method string, ok bool)
}

func NewFlow(cfg *config.Config, store *Store) *Flow {
	return &Flow{cfg: cfg, Client: HTTPClient(cfg.IDP), Store: store, cache: newIDCache(),
		oa: oauthApp(cfg, cfg.IDP.ClientID, cfg.ConsoleHost), ops: oauthApp(cfg, cfg.Operator.ClientID, cfg.Operator.Host)}
}

// oauthApp is one PKCE app of the IdP, called back on https://<host>/auth/callback.
func oauthApp(cfg *config.Config, clientID, host string) oauth2.Config {
	iss := strings.TrimRight(cfg.IDP.Issuer, "/")
	return oauth2.Config{
		ClientID:    clientID,
		Endpoint:    oauth2.Endpoint{AuthURL: iss + "/oauth/v2/authorize", TokenURL: iss + "/oauth/v2/token", AuthStyle: oauth2.AuthStyleInParams},
		RedirectURL: "https://" + host + "/auth/callback",
		Scopes: []string{"openid", "profile", "email",
			cfg.IDP.URN("user:resourceowner"), cfg.IDP.APIAudience(), cfg.IDP.URN("org:project:roles")},
	}
}

func (f *Flow) Login(w http.ResponseWriter, r *http.Request) {
	aud, oa := f.app(r)
	if oa.ClientID == "" {
		// With no IdP there is nothing to re-authenticate at; the local sign-in page is the way back.
		if r.URL.Query().Get("reauth") == "1" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		Problem(w, http.StatusServiceUnavailable, "Sign-in for this app isn't set up yet.")
		return
	}
	var prov config.IdPProvider
	if id := r.URL.Query().Get("idp"); id != "" {
		var ok bool
		if prov, ok = f.cfg.IDP.Provider(id); !ok {
			Problem(w, http.StatusBadRequest, "Unknown sign-in provider.")
			return
		}
	}
	v := oauth2.GenerateVerifier()
	st := RandomString(16)
	rd := r.URL.Query().Get("rd")
	if !strings.HasPrefix(rd, "/") || strings.HasPrefix(rd, "//") {
		rd = "/"
	}
	id, err := f.Store.New(r.Context(), &Session{Verifier: v, State: st, ReturnTo: rd, Aud: aud, Expires: time.Now().Add(10 * time.Minute)})
	if err != nil {
		Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	setCookie(w, flowCookie, id, 600)
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(v)}
	if r.URL.Query().Get("reauth") == "1" {
		// Fresh credentials required (break-glass): ignore any IdP session.
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "login"), oauth2.SetAuthURLParam("max_age", "0"))
	}
	if prov.IdPID != "" {
		// Zitadel skips its own login page and redirects to the upstream provider.
		opts = append(opts, oauth2.SetAuthURLParam("scope", idpScope(oa.Scopes, f.cfg.IDP.URN("org:idp:id:"+prov.IdPID))))
	}
	if hint := r.URL.Query().Get("login_hint"); hint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", hint))
	}
	http.Redirect(w, r, oa.AuthCodeURL(st, opts...), http.StatusFound)
}

func (f *Flow) Callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		http.Redirect(w, r, "/login?error=expired", http.StatusFound)
		return
	}
	pending := f.Store.Get(r.Context(), c.Value)
	f.Store.Delete(r.Context(), c.Value)
	setCookie(w, flowCookie, "", -1)
	aud, oa := f.app(r)
	if pending == nil || r.URL.Query().Get("state") != pending.State || pending.Aud != aud {
		http.Redirect(w, r, "/login?error=state", http.StatusFound)
		return
	}
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, f.Client)
	tok, err := oa.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(pending.Verifier))
	if err != nil {
		http.Redirect(w, r, "/login?error=exchange", http.StatusFound)
		return
	}
	id, err := f.Store.New(r.Context(), &Session{AccessToken: tok.AccessToken, Expires: tok.Expiry, AuthAt: authTime(tok), Aud: aud})
	if err != nil {
		http.Redirect(w, r, "/login?error=exchange", http.StatusFound)
		return
	}
	setCookie(w, cookieName, id, int(time.Until(tok.Expiry).Seconds()))
	http.Redirect(w, r, pending.ReturnTo, http.StatusFound)
}

func (f *Flow) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		f.Store.Delete(r.Context(), c.Value)
	}
	setCookie(w, cookieName, "", -1)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func setCookie(w http.ResponseWriter, name, val string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: val, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

// idpScope is the app's scopes plus the IdP hint scope, which sends the user straight to that
// upstream provider (Google, GitHub...) instead of the IdP's own login page.
func idpScope(base []string, hint string) string {
	return strings.Join(append(append([]string{}, base...), hint), " ")
}
