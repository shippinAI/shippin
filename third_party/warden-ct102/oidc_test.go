package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOIDCConfigFromEnvBuildsDiscoveryURL(t *testing.T) {
	t.Setenv("WARDEN_AUTHENTIK_ISSUER", "https://auth.example.invalid/application/o/warden")
	t.Setenv("WARDEN_AUTHENTIK_CLIENT_ID", "warden")

	cfg := getOIDCConfig(nil)
	if !cfg.Enabled {
		t.Fatal("expected OIDC to be enabled")
	}
	if !cfg.PAMEnabled {
		t.Fatal("expected PAM to remain enabled by default")
	}
	want := "https://auth.example.invalid/application/o/warden/.well-known/openid-configuration"
	if cfg.DiscoveryURL != want {
		t.Fatalf("discovery url = %q, want %q", cfg.DiscoveryURL, want)
	}
}

func TestHandleOIDCStartRedirectsToProvider(t *testing.T) {
	provider := newOIDCTestProvider(t)
	t.Setenv("WARDEN_AUTHENTIK_ISSUER", provider.issuer)
	t.Setenv("WARDEN_AUTHENTIK_CLIENT_ID", "warden")
	t.Setenv("WARDEN_AUTHENTIK_DISCOVERY_URL", provider.discoveryURL)
	t.Setenv("WARDEN_PUBLIC_URL", "https://warden.example.invalid")
	resetOIDCStatesForTest()

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/start?return_to=/workspaces", nil)
	rec := httptest.NewRecorder()
	handleOIDCStart(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/authorize" {
		t.Fatalf("redirect path = %q", u.Path)
	}
	q := u.Query()
	for _, key := range []string{"state", "nonce", "code_challenge"} {
		if q.Get(key) == "" {
			t.Fatalf("expected %s in redirect query", key)
		}
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected code challenge method: %q", q.Get("code_challenge_method"))
	}
	if q.Get("redirect_uri") != "https://warden.example.invalid/auth/oidc/callback" {
		t.Fatalf("unexpected redirect uri: %q", q.Get("redirect_uri"))
	}
}

func TestHandleOIDCCallbackCreatesSessionFromUserInfo(t *testing.T) {
	provider := newOIDCTestProvider(t)
	t.Setenv("WARDEN_AUTHENTIK_ISSUER", provider.issuer)
	t.Setenv("WARDEN_AUTHENTIK_CLIENT_ID", "warden")
	t.Setenv("WARDEN_AUTHENTIK_DISCOVERY_URL", provider.discoveryURL)
	t.Setenv("WARDEN_PUBLIC_URL", "https://warden.example.invalid")
	resetOIDCStatesForTest()

	startReq := httptest.NewRequest(http.MethodGet, "/auth/oidc/start?return_to=/workspaces", nil)
	startRec := httptest.NewRecorder()
	handleOIDCStart(startRec, startReq)
	redirectURL, err := url.Parse(startRec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := redirectURL.Query().Get("state")
	if state == "" {
		t.Fatal("missing state from OIDC start redirect")
	}

	cbReq := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+url.QueryEscape(state)+"&code=test-code", nil)
	cbRec := httptest.NewRecorder()
	handleOIDCCallback(cbRec, cbReq)

	if cbRec.Code != http.StatusSeeOther {
		t.Fatalf("expected callback redirect, got %d body=%q", cbRec.Code, cbRec.Body.String())
	}
	if got := cbRec.Header().Get("Location"); got != "/workspaces" {
		t.Fatalf("callback redirect = %q", got)
	}
	cookies := cbRec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName {
		t.Fatalf("expected %s cookie, got %+v", sessionCookieName, cookies)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	user, ok := getSession(req)
	if !ok || user != "hades" {
		t.Fatalf("session user = %q ok=%v", user, ok)
	}
	if _, _, ok := getSessionPassword(req); ok {
		t.Fatal("OIDC sessions must not expose a PAM password for sudo")
	}
}

type oidcTestProvider struct {
	server       *httptest.Server
	issuer       string
	discoveryURL string
}

func newOIDCTestProvider(t *testing.T) oidcTestProvider {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/application/o/warden/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 server.URL + "/application/o/warden",
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"userinfo_endpoint":      server.URL + "/userinfo",
			})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("code") != "test-code" || r.FormValue("code_verifier") == "" {
				http.Error(w, "bad token request", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"id_token":     "test.id.token",
			})
		case "/userinfo":
			if !strings.EqualFold(r.Header.Get("Authorization"), "Bearer access-token") {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"sub":                "authentik-user-hades",
				"preferred_username": "hades",
				"email":              "hades@example.test",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return oidcTestProvider{
		server:       server,
		issuer:       server.URL + "/application/o/warden",
		discoveryURL: server.URL + "/application/o/warden/.well-known/openid-configuration",
	}
}
