package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const oidcStateTTL = 10 * time.Minute

type OIDCConfig struct {
	Enabled      bool
	PAMEnabled   bool
	IssuerURL    string
	ClientID     string
	ClientSecret string
	DiscoveryURL string
	RedirectURL  string
	Scope        string
}

type oidcState struct {
	Nonce        string
	CodeVerifier string
	ReturnTo     string
	RedirectURL  string
	Expires      time.Time
}

var oidcStates = struct {
	sync.Mutex
	values map[string]oidcState
}{values: make(map[string]oidcState)}

func getOIDCConfig(r *http.Request) OIDCConfig {
	issuer := strings.TrimRight(os.Getenv("WARDEN_AUTHENTIK_ISSUER"), "/")
	clientID := os.Getenv("WARDEN_AUTHENTIK_CLIENT_ID")
	discoveryURL := strings.TrimSpace(os.Getenv("WARDEN_AUTHENTIK_DISCOVERY_URL"))
	if discoveryURL == "" && issuer != "" {
		discoveryURL = issuer + "/.well-known/openid-configuration"
	}
	scope := strings.TrimSpace(os.Getenv("WARDEN_AUTHENTIK_SCOPES"))
	if scope == "" {
		scope = "openid profile email"
	}
	return OIDCConfig{
		Enabled:      issuer != "" && clientID != "",
		PAMEnabled:   envBoolDefault("WARDEN_AUTH_PAM_ENABLED", true),
		IssuerURL:    issuer,
		ClientID:     clientID,
		ClientSecret: os.Getenv("WARDEN_AUTHENTIK_CLIENT_SECRET"),
		DiscoveryURL: discoveryURL,
		RedirectURL:  publicBaseURL(r) + "/auth/oidc/callback",
		Scope:        scope,
	}
}

func envBoolDefault(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	return raw != "false" && raw != "0" && raw != "off"
}

func publicBaseURL(r *http.Request) string {
	if base := strings.TrimRight(os.Getenv("WARDEN_PUBLIC_URL"), "/"); base != "" {
		return base
	}
	if r == nil || r.Host == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.Split(proto, ",")[0]
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = strings.Split(forwardedHost, ",")[0]
	}
	return scheme + "://" + strings.TrimSpace(host)
}

func resetOIDCStatesForTest() {
	oidcStates.Lock()
	oidcStates.values = make(map[string]oidcState)
	oidcStates.Unlock()
}

func createOIDCSession(username, subject string) string {
	token, err := newOIDCRandom()
	if err != nil {
		return ""
	}
	now := time.Now()
	sessionsMu.Lock()
	sessions[token] = session{
		username:     username,
		authMethod:   "oidc",
		subject:      subject,
		createdAt:    now,
		lastActivity: now,
	}
	sessionsMu.Unlock()
	return token
}

func newOIDCState(r *http.Request, cfg OIDCConfig) (string, oidcState, error) {
	stateToken, err := newOIDCRandom()
	if err != nil {
		return "", oidcState{}, err
	}
	nonce, err := newOIDCRandom()
	if err != nil {
		return "", oidcState{}, err
	}
	verifier, err := newOIDCRandom()
	if err != nil {
		return "", oidcState{}, err
	}
	return stateToken, oidcState{
		Nonce:        nonce,
		CodeVerifier: verifier,
		ReturnTo:     safeReturnTo(r.URL.Query().Get("return_to")),
		RedirectURL:  cfg.RedirectURL,
		Expires:      time.Now().Add(oidcStateTTL),
	}, nil
}

func saveOIDCState(token string, state oidcState) {
	oidcStates.Lock()
	oidcStates.values[token] = state
	oidcStates.Unlock()
}

func popOIDCState(token string) (oidcState, bool) {
	oidcStates.Lock()
	defer oidcStates.Unlock()
	state, ok := oidcStates.values[token]
	if !ok || time.Now().After(state.Expires) {
		delete(oidcStates.values, token)
		return oidcState{}, false
	}
	delete(oidcStates.values, token)
	return state, true
}

func safeReturnTo(raw string) string {
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	return "/"
}

func newOIDCRandom() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func oidcCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
