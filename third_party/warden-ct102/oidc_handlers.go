package main

import (
	"log"
	"net/http"
	"net/url"
)

func handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := getOIDCConfig(r)
	if !cfg.Enabled {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	metadata, err := fetchOIDCMetadata(r.Context(), cfg)
	if err != nil {
		log.Printf("oidc metadata error: %v", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	stateToken, state, err := newOIDCState(r, cfg)
	if err != nil {
		log.Printf("oidc state error: %v", err)
		http.Error(w, "OIDC state unavailable", http.StatusInternalServerError)
		return
	}
	saveOIDCState(stateToken, state)

	authURL, err := url.Parse(metadata.AuthorizationEndpoint)
	if err != nil {
		http.Error(w, "OIDC provider misconfigured", http.StatusBadGateway)
		return
	}
	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", cfg.RedirectURL)
	q.Set("scope", cfg.Scope)
	q.Set("state", stateToken)
	q.Set("nonce", state.Nonce)
	q.Set("code_challenge", oidcCodeChallenge(state.CodeVerifier))
	q.Set("code_challenge_method", "S256")
	authURL.RawQuery = q.Encode()
	http.Redirect(w, r, authURL.String(), http.StatusSeeOther)
}

func handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		log.Printf("oidc callback error: %s", errMsg)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	state, ok := popOIDCState(r.URL.Query().Get("state"))
	if !ok {
		http.Error(w, "Invalid OIDC state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing OIDC code", http.StatusBadRequest)
		return
	}
	cfg := getOIDCConfig(r)
	cfg.RedirectURL = state.RedirectURL
	metadata, err := fetchOIDCMetadata(r.Context(), cfg)
	if err != nil {
		http.Error(w, "OIDC provider unavailable", http.StatusBadGateway)
		return
	}
	token, err := exchangeOIDCCode(r.Context(), cfg, metadata, state, code)
	if err != nil {
		log.Printf("oidc token exchange error: %v", err)
		http.Error(w, "OIDC token exchange failed", http.StatusBadGateway)
		return
	}
	userInfo, err := fetchOIDCUserInfo(r.Context(), metadata, token.AccessToken)
	if err != nil {
		log.Printf("oidc userinfo error: %v", err)
		http.Error(w, "OIDC userinfo failed", http.StatusBadGateway)
		return
	}
	username := userInfo.Username()
	if username == "" || userInfo.Subject == "" {
		http.Error(w, "OIDC identity incomplete", http.StatusBadGateway)
		return
	}
	sessionToken := createOIDCSession(username, userInfo.Subject)
	setSessionCookie(w, sessionToken)
	log.Printf("user %q logged in with authentik from %s", username, getClientIP(r))
	addAuditEntry(username, "Login", "Logged in with Authentik", getClientIP(r), "auth")
	http.Redirect(w, r, state.ReturnTo, http.StatusSeeOther)
}
