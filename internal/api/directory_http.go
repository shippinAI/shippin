package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/identity"
)

func (a *API) registerDirectory(mux Router) {
	if a.directory == nil {
		return
	}
	mux.HandleFunc("POST /v1/setup", a.setupRedeem)
	mux.HandleFunc("POST /v1/session/password", a.passwordLogin)
	mux.HandleFunc("PUT /v1/directory/me/password", a.passwordChange)
	mux.HandleFunc("POST /v1/session/magic", a.magicBegin)
	mux.HandleFunc("POST /v1/session/magic/redeem", a.magicRedeem)
	mux.HandleFunc("POST /v1/session/recovery", a.recoveryRedeem)
	mux.HandleFunc("POST /v1/session/passkey/challenge", a.passkeyChallenge)
	mux.HandleFunc("GET /v1/session/passkeys", a.passkeyList)
	mux.HandleFunc("DELETE /v1/session/passkeys/{id}", a.passkeyDelete)
	mux.HandleFunc("POST /v1/session/passkey/register", a.passkeyRegister)
	mux.HandleFunc("POST /v1/session/passkey/login", a.passkeyLogin)
	mux.HandleFunc("POST /v1/invites/redeem", a.inviteRedeem)
	mux.HandleFunc("POST /v1/waitlist", a.waitlistJoin)
	mux.HandleFunc("POST /v1/waitlist/confirm", a.waitlistConfirm)
	mux.HandleFunc("GET /v1/directory/me", a.directoryMe)
	mux.HandleFunc("GET /v1/admin/waitlist", a.admin(a.waitlistList, "waitlist:read"))
	mux.HandleFunc("GET /v1/admin/waitlist.csv", a.admin(a.waitlistCSV, "waitlist:read"))
	mux.HandleFunc("POST /v1/admin/waitlist/invite", a.admin(a.waitlistInvite, "waitlist:write"))
	mux.HandleFunc("POST /v1/admin/invites", a.admin(a.inviteCreate, "invite:write"))
	mux.HandleFunc("GET /v1/admin/config", a.admin(a.adminConfig, "config:read"))
	mux.HandleFunc("PUT /v1/admin/branding", a.admin(a.adminBrand, "config:write"))
	mux.HandleFunc("POST /v1/admin/api-keys", a.admin(a.adminKey, "config:write"))
	mux.HandleFunc("POST /v1/admin/webhooks", a.admin(a.adminHook, "config:write"))
	mux.HandleFunc("GET /.well-known/openid-configuration", a.oidcDiscovery)
	mux.HandleFunc("GET /oauth/authorize", a.oidcAuthorize)
	mux.HandleFunc("POST /oauth/token", a.oidcToken)
	mux.HandleFunc("GET /oauth/userinfo", a.oidcUserinfo)
	mux.HandleFunc("GET /auth/upstream/{id}", a.upstreamStart)
	mux.HandleFunc("GET /auth/upstream/callback", a.upstreamCallback)
}

func (a *API) localUser(r *http.Request) (string, bool) {
	s := a.flow.Current(r)
	if s == nil || s.UserID == "" {
		return "", false
	}
	return s.UserID, true
}

func (a *API) admin(next func(http.ResponseWriter, *http.Request, string), scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id, ok := a.localUser(r); ok {
			var role string
			_ = a.db.Pool.QueryRow(r.Context(), `SELECT role FROM identity_users WHERE id = $1`, id).Scan(&role)
			if role == "owner" || role == "admin" {
				next(w, r, id)
				return
			}
		}
		raw := authHeader(r)
		scopes, err := a.directory.KeyScopes(r.Context(), raw)
		if err != nil || !identity.HasScope(scopes, scope) {
			auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		next(w, r, "api-key")
	}
}

func authHeader(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return ""
}

func (a *API) startSession(w http.ResponseWriter, r *http.Request, id, method string) {
	sid, err := a.flow.StartLocal(w, r, id, method)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	writeJSON(w, map[string]string{"user_id": id, "session": sid})
}

func (a *API) setupRedeem(w http.ResponseWriter, r *http.Request) {
	var in identity.SetupInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	u, codes, err := a.directory.RedeemSetup(r.Context(), in)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrSetupUsed):
			auth.Problem(w, http.StatusUnauthorized, "Setup code is not valid or has expired.")
		case errors.Is(err, identity.ErrOwnerExists):
			auth.Problem(w, http.StatusConflict, "This installation already has an owner. Sign in to continue.")
		case errors.Is(err, identity.ErrSetupInput), errors.Is(err, identity.ErrPassword), errors.Is(err, identity.ErrBrand):
			auth.Problem(w, http.StatusBadRequest, err.Error())
		default:
			log.Printf("setup transaction: %v", err)
			auth.Problem(w, http.StatusServiceUnavailable, "Setup could not be saved. Try again.")
		}
		return
	}
	sid, err := a.flow.StartLocal(w, r, u.ID, "setup")
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"user_id": u.ID, "session": sid, "recovery_codes": codes})
}

func (a *API) passwordLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	u, err := a.directory.AuthenticatePassword(r.Context(), in.Identifier, in.Password, clientIP(r))
	if err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Username or password is incorrect.")
		return
	}
	a.startSession(w, r, u.ID, "password")
}

func (a *API) passwordChange(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.localUser(r)
	if !ok {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	var in struct {
		Current  string `json:"current_password"`
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	if err := a.directory.SetPassword(r.Context(), uid, in.Current, in.Password, clientIP(r)); err != nil {
		if errors.Is(err, identity.ErrRateLimited) {
			auth.Problem(w, http.StatusTooManyRequests, "Wait before trying another password change.")
			return
		}
		if errors.Is(err, identity.ErrNotFound) {
			auth.Problem(w, http.StatusUnauthorized, "The current password was not accepted, or the account is unavailable.")
			return
		}
		auth.Problem(w, http.StatusBadRequest, "Choose a password with at least 12 characters.")
		return
	}
	writeJSON(w, map[string]string{"status": "updated"})
}

func (a *API) magicBegin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Invite string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	if err := a.directory.RequestMagic(r.Context(), in.Email, in.Invite); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

func (a *API) magicRedeem(w http.ResponseWriter, r *http.Request) {
	var in struct{ Token string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	u, err := a.directory.RedeemMagic(r.Context(), in.Token)
	if err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Sign-in link is not valid.")
		return
	}
	a.startSession(w, r, u.ID, "email_link")
}

func (a *API) recoveryRedeem(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Code string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	u, err := a.directory.RedeemRecovery(r.Context(), in.Email, in.Code)
	if err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Recovery code is not valid.")
		return
	}
	a.startSession(w, r, u.ID, "recovery_code")
}

// passkeyChallenge starts a ceremony. The browser signs what this panel hands it, and nothing else counts.
func (a *API) passkeyChallenge(w http.ResponseWriter, r *http.Request) {
	raw, err := a.directory.NewPasskeyChallenge(r.Context(), clientIP(r))
	if errors.Is(err, identity.ErrRateLimited) {
		auth.Problem(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	if err != nil {
		log.Printf("passkey challenge: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "Passkeys are unavailable right now.")
		return
	}
	writeJSON(w, map[string]string{"challenge": base64.RawURLEncoding.EncodeToString(raw), "rp_id": a.cfg.Directory.RPID, "rp_name": a.cfg.Brand.Name})
}

// spendChallenge takes the challenge a browser says it answered. It works once.
func (a *API) spendChallenge(ctx context.Context, encoded string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 {
		return nil, identity.ErrChallenge
	}
	return raw, a.directory.UsePasskeyChallenge(ctx, raw)
}

func (a *API) passkeyList(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.localUser(r)
	if !ok {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	items, err := a.directory.ListPasskeys(r.Context(), uid)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Passkeys are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.localUser(r)
	if !ok {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	if err := a.directory.DeletePasskey(r.Context(), uid, r.PathValue("id")); err != nil {
		auth.Problem(w, http.StatusNotFound, "That passkey is not yours, or is already gone.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) passkeyRegister(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.localUser(r)
	if !ok {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	var in struct {
		Challenge         string `json:"challenge"`
		ClientDataJSON    []byte `json:"client_data_json"`
		AuthenticatorData []byte `json:"authenticator_data"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	challenge, err := a.spendChallenge(r.Context(), in.Challenge)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Passkey was rejected.")
		return
	}
	key, err := identity.ParseRegistration(identity.Registration{ClientDataJSON: in.ClientDataJSON, AuthenticatorData: in.AuthenticatorData}, challenge, a.cfg.Directory.Origin, a.cfg.Directory.RPID)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Passkey was rejected.")
		return
	}
	if err := a.directory.SavePasskey(r.Context(), uid, key); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Passkey was rejected.")
		return
	}
	writeJSON(w, map[string]string{"status": "saved"})
}

func (a *API) passkeyLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email             string `json:"email"`
		Challenge         string `json:"challenge"`
		CredentialID      []byte `json:"credential_id"`
		ClientDataJSON    []byte `json:"client_data_json"`
		AuthenticatorData []byte `json:"authenticator_data"`
		Signature         []byte `json:"signature"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	challenge, err := a.spendChallenge(r.Context(), in.Challenge)
	if err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Passkey was rejected.")
		return
	}
	uid, keys, err := a.directory.Passkeys(r.Context(), in.Email)
	if err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Passkey was rejected.")
		return
	}
	var match identity.Passkey
	for _, k := range keys {
		if string(k.CredentialID) == string(in.CredentialID) {
			match = k
		}
	}
	count, err := identity.VerifyAssertion(match, identity.Assertion{CredentialID: in.CredentialID, ClientDataJSON: in.ClientDataJSON, AuthenticatorData: in.AuthenticatorData, Signature: in.Signature}, challenge, a.cfg.Directory.Origin, a.cfg.Directory.RPID)
	if err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Passkey was rejected.")
		return
	}
	_ = a.directory.BumpPasskey(r.Context(), in.CredentialID, count)
	a.startSession(w, r, uid, "passkey")
}

func (a *API) inviteRedeem(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code, Email, Name string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	u, codes, err := a.directory.RedeemInvite(r.Context(), in.Code, in.Email, in.Name)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Invite is not valid.")
		return
	}
	sid, err := a.flow.StartLocal(w, r, u.ID, "invite")
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"user_id": u.ID, "session": sid, "recovery_codes": codes})
}

func (a *API) waitlistJoin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Name, Company, Notes, Captcha string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	if err := a.directory.JoinWaitlist(r.Context(), in.Email, in.Name, in.Company, in.Notes, in.Captcha, clientIP(r)); err != nil {
		code := http.StatusBadRequest
		switch {
		case err.Error() == "rate limit":
			code = http.StatusTooManyRequests
		case errors.Is(err, identity.ErrWaitlistClosed):
			code = http.StatusForbidden
		case errors.Is(err, identity.ErrNoMail):
			code = http.StatusServiceUnavailable
		}
		auth.Problem(w, code, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "pending"})
}

func (a *API) waitlistConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct{ Token string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	if err := a.directory.ConfirmWaitlist(r.Context(), in.Token); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Confirmation link is not valid.")
		return
	}
	writeJSON(w, map[string]string{"status": "confirmed"})
}

func (a *API) directoryMe(w http.ResponseWriter, r *http.Request) {
	id, ok := a.localUser(r)
	if !ok {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	var email, name, role string
	if err := a.db.Pool.QueryRow(r.Context(), `SELECT email, name, role FROM identity_users WHERE id = $1 AND active`, id).Scan(&email, &name, &role); err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	writeJSON(w, map[string]string{"user_id": id, "email": email, "name": name, "role": role})
}

func (a *API) waitlistList(w http.ResponseWriter, r *http.Request, _ string) {
	rows, err := a.directory.SearchWaitlist(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Waitlist could not be read.")
		return
	}
	writeJSON(w, map[string]any{"items": rows})
}

func (a *API) waitlistCSV(w http.ResponseWriter, r *http.Request, _ string) {
	body, err := a.directory.ExportWaitlist(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Waitlist could not be exported.")
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	_, _ = w.Write([]byte(body))
}

func (a *API) waitlistInvite(w http.ResponseWriter, r *http.Request, actor string) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	codes, err := a.directory.BulkInvite(r.Context(), actor, in.IDs)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"invites": codes})
}

func (a *API) inviteCreate(w http.ResponseWriter, r *http.Request, actor string) {
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
		Uses  int    `json:"uses"`
		Hours int    `json:"hours"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	if in.Hours <= 0 {
		in.Hours = 24 * 14
	}
	code, err := a.directory.CreateInvite(r.Context(), actor, in.Email, in.Role, in.Uses, time.Now().Add(time.Duration(in.Hours)*time.Hour))
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Invite could not be created.")
		return
	}
	writeJSON(w, map[string]string{"invite": code})
}

func mergeBrand(base, over config.Brand) config.Brand {
	if over.Name != "" {
		base.Name = over.Name
	}
	if over.Short != "" {
		base.Short = over.Short
	}
	if over.Tagline != "" {
		base.Tagline = over.Tagline
	}
	if over.LogoURL != "" {
		base.LogoURL = over.LogoURL
	}
	if over.Color != "" {
		base.Color = over.Color
	}
	if over.PrivacyURL != "" {
		base.PrivacyURL = over.PrivacyURL
	}
	if over.TermsURL != "" {
		base.TermsURL = over.TermsURL
	}
	return base
}

func (a *API) adminConfig(w http.ResponseWriter, r *http.Request, _ string) {
	brand := a.cfg.Brand
	if over, ok := a.directory.Branding(r.Context()); ok {
		brand = mergeBrand(brand, over)
	}
	writeJSON(w, map[string]any{"version": a.directory.ConfigVersion(r.Context()), "signup": a.signupMode(r.Context()), "brand": brand, "upstream_required": a.cfg.Directory.UpstreamRequired})
}

func (a *API) adminBrand(w http.ResponseWriter, r *http.Request, actor string) {
	var in config.Brand
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	if err := a.directory.SaveBranding(r.Context(), in); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Branding could not be saved.")
		return
	}
	a.directory.NoteConfigWrite(r.Context(), actor)
	writeJSON(w, map[string]int{"version": a.directory.ConfigVersion(r.Context())})
}

func (a *API) adminKey(w http.ResponseWriter, r *http.Request, _ string) {
	var in struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Name == "" || len(in.Scopes) == 0 {
		auth.Problem(w, http.StatusBadRequest, "Name and scopes are required.")
		return
	}
	key, err := a.directory.CreateAPIKey(r.Context(), in.Name, in.Scopes)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "API key could not be created.")
		return
	}
	writeJSON(w, map[string]any{"key": key, "scopes": in.Scopes})
}

func (a *API) adminHook(w http.ResponseWriter, r *http.Request, _ string) {
	var in struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.URL == "" || len(in.Events) == 0 {
		auth.Problem(w, http.StatusBadRequest, "URL and events are required.")
		return
	}
	secret, err := a.directory.AddWebhook(r.Context(), in.URL, in.Events)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Webhook could not be saved.")
		return
	}
	writeJSON(w, map[string]string{"secret": secret})
}

func (a *API) oidcDiscovery(w http.ResponseWriter, r *http.Request) {
	origin := a.cfg.Directory.Origin
	writeJSON(w, map[string]any{
		"issuer": origin, "authorization_endpoint": origin + "/oauth/authorize", "token_endpoint": origin + "/oauth/token",
		"userinfo_endpoint": origin + "/oauth/userinfo", "response_types_supported": []string{"code"},
		"grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"},
		"subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{},
	})
}

func (a *API) oidcAuthorize(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.localUser(r)
	if !ok {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	q := r.URL.Query()
	if q.Get("code_challenge_method") != "S256" {
		auth.Problem(w, http.StatusBadRequest, "PKCE S256 is required.")
		return
	}
	code, err := a.directory.AuthorizeCode(r.Context(), q.Get("client_id"), uid, q.Get("redirect_uri"), q.Get("code_challenge"))
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+q.Get("state"), http.StatusFound)
}

func (a *API) oidcToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Request body was not accepted.")
		return
	}
	var refresh string
	var u identity.User
	var err error
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		refresh, u, err = a.directory.ExchangeCode(r.Context(), r.PostForm.Get("code"), r.PostForm.Get("code_verifier"), r.PostForm.Get("client_id"), r.PostForm.Get("redirect_uri"))
	case "refresh_token":
		refresh, u, err = a.directory.RotateRefresh(r.Context(), r.PostForm.Get("refresh_token"), r.PostForm.Get("client_id"))
	default:
		err = errors.New("unsupported grant")
	}
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"access_token": "local:" + u.ID, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600})
}

func (a *API) oidcUserinfo(w http.ResponseWriter, r *http.Request) {
	raw := authHeader(r)
	if len(raw) < 6 || raw[:6] != "local:" {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	var email, name string
	if err := a.db.Pool.QueryRow(r.Context(), `SELECT email, name FROM identity_users WHERE id = $1`, raw[6:]).Scan(&email, &name); err != nil {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	writeJSON(w, map[string]string{"sub": raw[6:], "email": email, "name": name, "email_verified": "true"})
}

func (a *API) upstreamStart(w http.ResponseWriter, r *http.Request) {
	u, ok := identity.FindUpstream(a.cfg, r.PathValue("id"))
	if !ok {
		auth.Problem(w, http.StatusNotFound, "That sign-in provider is not configured.")
		return
	}
	d, err := identity.Discover(r.Context(), http.DefaultClient, u.Issuer)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "The identity provider could not be discovered.")
		return
	}
	verifier, err := identity.NewToken()
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	state, err := identity.NewToken()
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")
		return
	}
	redirect := identity.RedirectFor(a.cfg.Directory.Origin)
	http.SetCookie(w, &http.Cookie{Name: "shippin_upstream", Value: u.ID + "|" + state + "|" + verifier, Path: "/", MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, identity.UpstreamAuthURL(d, u, redirect, state, verifier), http.StatusFound)
}

func (a *API) upstreamCallback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("shippin_upstream")
	if err != nil {
		http.Redirect(w, r, "/login?error=state", http.StatusFound)
		return
	}
	parts := split3(c.Value)
	if parts[1] == "" || parts[1] != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=state", http.StatusFound)
		return
	}
	u, ok := identity.FindUpstream(a.cfg, parts[0])
	if !ok {
		http.Redirect(w, r, "/login?error=exchange", http.StatusFound)
		return
	}
	d, err := identity.Discover(r.Context(), http.DefaultClient, u.Issuer)
	if err != nil {
		http.Redirect(w, r, "/login?error=exchange", http.StatusFound)
		return
	}
	info, err := identity.ExchangeUpstream(r.Context(), http.DefaultClient, d, u, identity.RedirectFor(a.cfg.Directory.Origin), r.URL.Query().Get("code"), parts[2], os.Getenv(u.SecretEnv))
	if err != nil {
		http.Redirect(w, r, "/login?error=exchange", http.StatusFound)
		return
	}
	user, err := a.directory.LinkUpstream(r.Context(), info.Issuer, info.Sub, info.Email, info.EmailVerified)
	if err != nil {
		http.Redirect(w, r, "/login?error=exchange", http.StatusFound)
		return
	}
	if _, err := a.flow.StartLocal(w, r, user.ID, "identity_provider"); err != nil {
		http.Redirect(w, r, "/login?error=exchange", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func split3(s string) [3]string {
	var out [3]string
	n := 0
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '|' {
			if n < 3 {
				out[n] = s[start:i]
			}
			n++
			start = i + 1
		}
	}
	return out
}
