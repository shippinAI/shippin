package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agenthub"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/servers"
)

func (a *API) infraSystemIdentity(w http.ResponseWriter, r *http.Request, _ *caller) {
	if a.db == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "System identity is unavailable right now.")
		return
	}
	v, err := a.serverStore().Identity(r.Context(), r.PathValue("id"))
	if errors.Is(err, servers.ErrNotFound) {
		writeHVProblem(w, http.StatusNotFound, "server_not_found", "The system was not found.", "", "")
		return
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "System identity is unavailable right now.")
		return
	}
	writeJSON(w, v)
}

func (a *API) infraSetSystemIdentity(w http.ResponseWriter, r *http.Request, c *caller) {
	if a.db == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "System identity is unavailable right now.")
		return
	}
	var in struct {
		Label             string   `json:"label"`
		Aliases           []string `json:"aliases"`
		Tags              []string `json:"tags"`
		AgentHubTenantID  string   `json:"agenthub_tenant_id"`
		AgentHubMachineID string   `json:"agenthub_machine_id"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	v, err := a.serverStore().SetIdentity(r.Context(), servers.SystemIdentity{
		ServerID: r.PathValue("id"), Label: in.Label, Aliases: in.Aliases, Tags: in.Tags,
		AgentHubTenantID: in.AgentHubTenantID, AgentHubMachineID: in.AgentHubMachineID,
	}, c.id.Sub)
	switch {
	case errors.Is(err, servers.ErrIdentityInvalid):
		writeHVProblem(w, http.StatusBadRequest, "invalid_system_identity", "Use at most 20 unique aliases and 40 unique tags; AgentHub tenant and machine IDs must be supplied together.", "", "")
	case errors.Is(err, servers.ErrIdentityConflict):
		writeHVProblem(w, http.StatusConflict, "agenthub_machine_mapped", "That AgentHub machine is already mapped to another Shippin system.", "", "")
	case errors.Is(err, servers.ErrNotFound):
		writeHVProblem(w, http.StatusNotFound, "server_not_found", "The system was not found or has been revoked.", "", "")
	case err != nil:
		auth.Problem(w, http.StatusServiceUnavailable, "System identity could not be saved.")
	default:
		writeJSON(w, v)
	}
}

func (a *API) infraTestSystemIdentity(w http.ResponseWriter, r *http.Request, c *caller) {
	if a.db == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "System identity is unavailable right now.")
		return
	}
	var in struct{}
	if !decodeStrict(w, r, &in) {
		return
	}
	identity, err := a.serverStore().Identity(r.Context(), r.PathValue("id"))
	if errors.Is(err, servers.ErrNotFound) {
		writeHVProblem(w, http.StatusNotFound, "server_not_found", "The system was not found.", "", "")
		return
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "System identity is unavailable right now.")
		return
	}
	if identity.AgentHubTenantID == "" || identity.AgentHubMachineID == "" {
		writeHVProblem(w, http.StatusBadRequest, "agenthub_mapping_required", "Save both AgentHub IDs before checking the mapping.", "", "")
		return
	}

	var mapping agenthub.SystemMapping
	checkErr := errors.New("AgentHub connection is not configured")
	connection, connErr := a.db.AgentHubConnection(r.Context())
	if connErr == nil && connection.BaseURL != "" && connection.HasCredential {
		var token string
		token, checkErr = a.agentHubToken(r.Context())
		if checkErr == nil {
			var client *agenthub.Client
			client, checkErr = agenthub.NewClient(agenthub.Config{BaseURL: connection.BaseURL, Token: token, Timeout: 5 * time.Second})
			if checkErr == nil {
				ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
				mapping, checkErr = client.CheckSystemMapping(ctx, identity.AgentHubTenantID, identity.AgentHubMachineID, identity.ServerID)
				cancel()
			}
		}
	}
	if connErr != nil && !errors.Is(connErr, db.ErrNotFound) {
		checkErr = errors.New("AgentHub settings are unavailable")
	}

	observation := db.AgentHubMappingCheck{
		ID: uuid.NewString(), ServerID: identity.ServerID, TenantID: identity.AgentHubTenantID,
		MachineID: identity.AgentHubMachineID, Outcome: "failed", Detail: "AgentHub mapping could not be verified.", CheckedBy: c.id.Sub,
	}
	if checkErr == nil {
		version := mapping.PolicyVersion
		observation.Outcome, observation.Detail = "verified", "AgentHub mapping verified; this is not deployment authorization."
		observation.PolicyVersion, observation.PolicySHA256 = &version, mapping.PolicySHA256
	}
	auditDetail := "Read-only AgentHub system mapping check recorded with redacted outcome."
	if _, err := a.db.RecordAgentHubMappingCheck(r.Context(), observation, teamEntry(r, c, "infra.agenthub.mapping.check", auditDetail)); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The mapping check could not be recorded.")
		return
	}
	latest, err := a.serverStore().Identity(r.Context(), identity.ServerID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The mapping check was recorded, but its status could not be reloaded.")
		return
	}
	writeJSON(w, map[string]any{"ok": checkErr == nil, "identity": latest, "detail": observation.Detail})
}
