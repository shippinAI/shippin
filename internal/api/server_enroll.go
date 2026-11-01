package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/servers"
)

type enrollIn struct {
	Token    string `json:"token"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
}

// short keeps host-reported strings bounded and on one line.
func short(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if rs := []rune(s); len(rs) > 120 {
		return string(rs[:120])
	}
	return s
}

// enrollServer: hostd trades a single-use token for its credential. Every failure looks the same.
func (a *API) enrollServer(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.Servers.Enabled {
		auth.Problem(w, http.StatusNotFound, "Servers aren't enabled.")
		return
	}
	var in enrollIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Send a JSON body with token, hostname and version.")
		return
	}
	ttl := time.Duration(a.cfg.Servers.CredentialTTLSeconds) * time.Second
	e, err := a.serverStore().Enroll(r.Context(), in.Token, short(in.Hostname), short(in.Version), ttl)
	if errors.Is(err, servers.ErrBadToken) {
		auth.Problem(w, http.StatusUnauthorized, "The enrollment token is invalid, used or expired. Issue a new one.")
		return
	} else if err != nil {
		log.Printf("server enroll: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "Enrollment is unavailable right now.")
		return
	}
	log.Printf("server %s enrolled from %s", e.ServerID, clientIP(r))
	writeStatus(w, http.StatusCreated, e)
}

type beatIn struct {
	Version string `json:"version"`
}

// serverHeartbeat: Authorization: Bearer <host credential>; each beat slides its expiry.
func (a *API) serverHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.Servers.Enabled {
		auth.Problem(w, http.StatusNotFound, "Servers aren't enabled.")
		return
	}
	cred, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	var in beatIn
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
	ttl := time.Duration(a.cfg.Servers.CredentialTTLSeconds) * time.Second
	id, exp, err := a.serverStore().Beat(r.Context(), strings.TrimSpace(cred), short(in.Version), ttl)
	if !ok || errors.Is(err, servers.ErrBadCred) {
		auth.Problem(w, http.StatusUnauthorized, "The host credential is invalid or expired. Re-enroll with a new token.")
		return
	} else if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Heartbeats are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"server_id": id, "expires_at": exp, "heartbeat_seconds": a.cfg.Servers.HeartbeatSeconds})
}
