package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/bridge"
	"shippin.cloud/console/internal/db"
)

// Device sign-in for the bridge (RFC 8628 shape). The CLI starts it unauthenticated, a signed-in
// user approves the user code in the console, and the CLI redeems the device code once for a
// bridge token bound to that account (IdP sub + email).
func (a *API) bridgeRoutes(mux Router) {
	mux.HandleFunc("POST /v1/bridge/device", a.bridgeStart)
	mux.HandleFunc("POST /v1/bridge/token", a.bridgeToken)
	mux.HandleFunc("GET /v1/bridge/device/{code}", a.flow.Require(a.bridgePending))
	mux.HandleFunc("POST /v1/bridge/device/{code}", a.flow.Require(a.bridgeDecide))
	mux.HandleFunc("GET /v1/me/bridge-sessions", a.flow.Require(a.bridgeSessions))
	mux.HandleFunc("DELETE /v1/me/bridge-sessions/{id}", a.flow.Require(a.bridgeRevoke))
	mux.HandleFunc("GET /v1/bridge/me", a.bridgeMe)
	mux.HandleFunc("GET /v1/operator/authz-shadow", a.flow.Require(a.authzShadowStats))
	mux.HandleFunc("POST /v1/bridge/workspaces/{id}/cert", a.bridgeCert)
	mux.HandleFunc("GET /v1/bridge/workspaces/{id}/relay", a.bridgeRelay)
}

func (a *API) bridgeOn(w http.ResponseWriter) bool {
	if a.n2.br == nil {
		auth.Problem(w, http.StatusNotFound, "Devspace access through the bridge isn't enabled.")
		return false
	}
	return true
}

func (a *API) bridgeStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.bridgeOn(w) {
		return
	}
	var in struct {
		ClientName string `json:"client_name"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
	name := strings.TrimSpace(in.ClientName)
	if name == "" || len(name) > 80 {
		name = "Shippin bridge"
	}
	ttl := time.Duration(a.n2.br.cfg.DeviceTTLSecond) * time.Second
	_, dev, code, err := a.n2.br.store.Start(r.Context(), name, ttl)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-in couldn't start.")
		return
	}
	uri := a.consoleOrigin() + "/device"
	writeJSON(w, map[string]any{"device_code": dev, "user_code": code, "verification_uri": uri,
		"verification_uri_complete": uri + "?code=" + code, "expires_in": int(ttl.Seconds()), "interval": a.n2.br.cfg.PollSeconds})
}

func (a *API) bridgeToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.bridgeOn(w) {
		return
	}
	var in struct {
		DeviceCode string `json:"device_code"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
	tok, ses, err := a.n2.br.store.Redeem(r.Context(), in.DeviceCode, a.n2.br.cfg.TokenPrefix)
	if err != nil {
		code := "expired_token"
		if errors.Is(err, bridge.ErrPending) || errors.Is(err, bridge.ErrDenied) {
			code = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"access_token": tok, "token_type": "Bearer", "expires_at": ses.ExpiresAt,
		"account": map[string]string{"id": ses.Sub, "email": ses.Email}, "session_id": ses.ID})
}

func (a *API) bridgePending(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.bridgeOn(w) {
		return
	}
	name, err := a.n2.br.store.Pending(r.Context(), bridge.NormalizeUserCode(r.PathValue("code")))
	if err != nil || name == "" {
		auth.Problem(w, http.StatusNotFound, "That code is unknown or expired. Start the sign-in again.")
		return
	}
	writeJSON(w, map[string]string{"client_name": name})
}

func (a *API) bridgeDecide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.bridgeOn(w) {
		return
	}
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	var in struct {
		Approve bool `json:"approve"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in)
	code := bridge.NormalizeUserCode(r.PathValue("code"))
	act := map[bool]string{true: "bridge.device_approved", false: "bridge.device_denied"}[in.Approve]
	if !a.auditFirst(w, r, db.AuditEntry{Actor: c.id.Sub, ActorName: c.id.Name, Action: act, Detail: "code " + code, IP: clientIP(r)}) {
		return
	}
	hours := time.Duration(a.n2.br.cfg.SessionHours) * time.Hour
	done, err := a.n2.br.store.Decide(r.Context(), code, in.Approve, c.id.Sub, c.id.Email, c.id, hours)
	if err != nil || !done {
		auth.Problem(w, http.StatusNotFound, "That code is unknown or expired. Start the sign-in again.")
		return
	}
	writeJSON(w, map[string]any{"approved": in.Approve})
}
