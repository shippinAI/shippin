package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/servers"
)

var serverCommandFailure = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,79}$`)

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
	if a.directory != nil {
		a.directory.EmitHostEnrolled(r.Context(), e.ServerID, short(in.Hostname))
	}
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
	command, err := a.serverStore().PendingCommand(r.Context(), id, strings.TrimSpace(cred))
	if err != nil {
		log.Printf("server heartbeat command read: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "Server commands are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"server_id": id, "expires_at": exp, "heartbeat_seconds": a.cfg.Servers.HeartbeatSeconds, "command": command})
}

type serverCommandResult struct {
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Failure    string `json:"failure_code,omitempty"`
}

// serverCommandResult accepts results only from the credential-bound target
// host. The command table, not host-supplied payload fields, selects the
// deployment generation that gets a reconcile wake-up.
func (a *API) serverCommandResult(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.Servers.Enabled {
		auth.Problem(w, http.StatusNotFound, "Servers aren't enabled.")
		return
	}
	cred, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	var in serverCommandResult
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if !ok || decoder.Decode(&in) != nil ||
		(in.Status != "passing" && in.Status != "failing") || in.DurationMS < 0 || in.DurationMS > 3_600_000 ||
		(in.Status == "passing" && in.Failure != "") ||
		(in.Status == "failing" && !serverCommandFailure.MatchString(in.Failure)) {
		auth.Problem(w, http.StatusBadRequest, "Send a valid bounded server command result.")
		return
	}
	result, _ := json.Marshal(in)
	completed, err := a.serverStore().CompleteCommand(r.Context(), strings.TrimSpace(cred), r.PathValue("commandId"), result)
	if errors.Is(err, servers.ErrBadCred) {
		auth.Problem(w, http.StatusUnauthorized, "The host credential is invalid, expired or not assigned this command.")
		return
	}
	if errors.Is(err, servers.ErrCommandConflict) {
		auth.Problem(w, http.StatusConflict, "The server command already has a different result.")
		return
	}
	if err != nil {
		log.Printf("server command result: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The server command result could not be stored.")
		return
	}
	if a.jobs == nil {
		log.Printf("server command result: deployment jobs unavailable")
		auth.Problem(w, http.StatusServiceUnavailable, "The result is stored and will be reconciled after the job queue recovers.")
		return
	}
	payload, err := json.Marshal(deploymentWakeup{DeploymentID: completed.DeploymentID, Generation: completed.Generation})
	if err != nil {
		auth.Problem(w, http.StatusInternalServerError, "The deployment wake-up could not be prepared.")
		return
	}
	targetID := completed.DeploymentID + ":" + strconv.FormatInt(completed.Generation, 10)
	job, _, err := a.jobs.Enqueue(r.Context(), jobs.Job{TenantID: completed.TenantID, Kind: jobs.KindDeploymentReconcile,
		TargetType: "deployment_generation", TargetID: targetID, MaxAttempts: a.cfg.Jobs.MaxAttempts,
		Payload: payload, CreatedBy: "hostd"})
	if err != nil {
		log.Printf("server command reconcile enqueue: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The deployment result is stored and will be reconciled on a later queue relay.")
		return
	}
	if a.runner != nil {
		a.runner.Wake()
	}
	writeStatus(w, http.StatusAccepted, map[string]any{"command_id": r.PathValue("commandId"), "job_id": job.ID, "accepted": true})
}
