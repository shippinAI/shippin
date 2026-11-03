package vaultixhost

import (
	"encoding/json"
	"net/http"
	"time"

	"shippin.cloud/console/internal/vaultix"
)

// keyring mints a keyring login for the vaultix-keyring CLI. The secret is shown once.
func (h *Handler) keyring(x call) {
	cid, sec, err := h.Svc.KeyringSecret(x.r.Context(), x.a, "keyring "+time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		upstream(x.w, err)
		return
	}
	x.w.Header().Set("Cache-Control", "no-store")
	reply(x.w, map[string]string{"api_url": h.Svc.PublicURL, "client_id": cid, "client_secret": sec, "workspace": x.a.Project, "workspace_slug": "ws-" + x.a.Sub})
}

// importCloud copies every secret of one Infisical Cloud (or any Infisical) environment into the
// caller's workspace, using a read-only machine identity the person made there. Nothing is kept.
func (h *Handler) importCloud(x call) {
	var in struct {
		URL          string `json:"url"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Project      string `json:"project"`
		FromEnv      string `json:"from_env"`
		ToEnv        string `json:"to_env"`
		Path         string `json:"path"`
	}
	if json.NewDecoder(http.MaxBytesReader(x.w, x.r.Body, 16<<10)).Decode(&in) != nil || in.URL == "" || in.Project == "" {
		problem(x.w, http.StatusBadRequest, "Give the source URL, project, environment and machine identity.")
		return
	}
	q := x.r.URL.Query()
	q.Set("env", in.ToEnv)
	x.r.URL.RawQuery = q.Encode()
	to, ok := h.scope(x)
	if !ok {
		return
	}
	src := vaultix.New(in.URL, in.ClientID, in.ClientSecret, h.Svc.Timeout)
	all, err := src.Pull(x.r.Context(), vaultix.Scope{Project: in.Project, Env: in.FromEnv, Path: in.Path})
	if err != nil {
		problem(x.w, http.StatusBadGateway, "The source rejected the export: check the identity and project.")
		return
	}
	n := 0
	for k, v := range all {
		if _, err := x.c.Set(x.r.Context(), to, k, v, true); err != nil {
			if _, err = x.c.Set(x.r.Context(), to, k, v, false); err != nil {
				upstream(x.w, err)
				return
			}
		}
		n++
	}
	reply(x.w, map[string]int{"imported": n})
}
