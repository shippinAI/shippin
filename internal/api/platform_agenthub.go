package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"shippin.cloud/console/internal/agenthub"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

const agentHubCredentialAAD = "shippin-agenthub-service-credential"

type agentHubConnectionInput struct {
	BaseURL         string `json:"base_url"`
	Credential      string `json:"credential,omitempty"`
	ClearCredential bool   `json:"clear_credential,omitempty"`
	Reason          string `json:"reason"`
}

func (a *API) opAgentHubConnection(w http.ResponseWriter, r *http.Request, _ *caller) {
	item, err := a.db.AgentHubConnection(r.Context())
	if err != nil && err != db.ErrNotFound {
		auth.Problem(w, http.StatusServiceUnavailable, "AgentHub settings are unavailable.")
		return
	}
	writeJSON(w, map[string]any{"base_url": item.BaseURL, "has_credential": item.HasCredential, "configured": item.BaseURL != "" && item.HasCredential})
}

func (a *API) opSetAgentHubConnection(w http.ResponseWriter, r *http.Request, c *caller) {
	var in agentHubConnectionInput
	if !decodeStrict(w, r, &in) || !reasonOK(w, in.Reason) {
		return
	}
	baseURL := strings.TrimSpace(in.BaseURL)
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(baseURL) > 2048 {
		auth.Problem(w, http.StatusBadRequest, "AgentHub URL must use HTTPS and contain no credentials, query or fragment.")
		return
	}
	old, err := a.db.AgentHubConnection(r.Context())
	if err != nil && err != db.ErrNotFound {
		auth.Problem(w, http.StatusServiceUnavailable, "AgentHub settings are unavailable.")
		return
	}
	if in.Credential != "" && in.ClearCredential {
		auth.Problem(w, http.StatusBadRequest, "Set a credential or clear it, not both.")
		return
	}
	var ciphertext []byte
	keep := in.Credential == "" && !in.ClearCredential && old.HasCredential
	if in.Credential != "" {
		sealer, sealErr := a.sealer()
		if sealErr != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "The installation cannot protect the AgentHub credential.")
			return
		}
		ciphertext, sealErr = sealer.Seal([]byte(in.Credential), agentHubCredentialAAD)
		if sealErr != nil {
			auth.Problem(w, http.StatusInternalServerError, "The AgentHub credential could not be protected.")
			return
		}
	}
	detail := "AgentHub endpoint updated; credential retained or replaced without recording its value. Reason: " + strings.TrimSpace(in.Reason)
	if in.ClearCredential {
		detail = "AgentHub endpoint updated and credential cleared. Reason: " + strings.TrimSpace(in.Reason)
	}
	e := teamEntry(r, c, "platform.agenthub.connection", detail)
	if err := a.db.SaveAgentHubConnection(r.Context(), baseURL, ciphertext, keep, c.id.Sub, e); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "AgentHub settings could not be saved.")
		return
	}
	writeJSON(w, map[string]any{"base_url": baseURL, "has_credential": keep || len(ciphertext) > 0, "configured": baseURL != "" && (keep || len(ciphertext) > 0)})
}

func (a *API) opTestAgentHubConnection(w http.ResponseWriter, r *http.Request, _ *caller) {
	var in struct {
		TenantID string `json:"tenant_id"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	item, err := a.db.AgentHubConnection(r.Context())
	if err != nil || !item.HasCredential || item.BaseURL == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "Configure an AgentHub HTTPS URL and credential first."})
		return
	}
	token, err := a.agentHubToken(r.Context())
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "The stored AgentHub credential could not be resolved."})
		return
	}
	client, err := agenthub.NewClient(agenthub.Config{BaseURL: item.BaseURL, Token: token, Timeout: 5 * time.Second})
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "The AgentHub connection settings are invalid."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	status, err := client.CheckPolicy(ctx, strings.TrimSpace(in.TenantID))
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "AgentHub policy could not be read for that tenant."})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "tenant_id": status.TenantID, "policy_version": status.Version, "policy_sha256": status.SHA256})
}

func (a *API) agentHubToken(ctx context.Context) (string, error) {
	item, err := a.db.AgentHubConnection(ctx)
	if err != nil {
		return "", err
	}
	if len(item.TokenCiphertext) == 0 {
		return "", errors.New("AgentHub credential is not configured")
	}
	sealer, err := a.sealer()
	if err != nil {
		return "", err
	}
	plain, err := sealer.Open(item.TokenCiphertext, agentHubCredentialAAD)
	return string(plain), err
}
