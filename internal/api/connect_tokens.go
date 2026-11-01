package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/vaultix"
)

// connectScope is the tenant's Vaultix folder for one workspace's provider tokens.
func (a *API) connectScope(ctx context.Context, tenant, ws string) (vaultix.Scope, bool) {
	vx := a.n2.vx
	if vx == nil {
		return vaultix.Scope{}, false
	}
	pid, err := vx.c.ProjectBySlug(ctx, vx.cfg.ProjectPrefix+tenant)
	if errors.Is(err, vaultix.ErrNotFound) || (err == nil && pid == "") {
		a.ensureVaultixWorkspace(tenant, ws) // tenants/workspaces from before auto-provisioning (B4)
		pid, err = vx.c.ProjectBySlug(ctx, vx.cfg.ProjectPrefix+tenant)
	}
	if err != nil || pid == "" {
		return vaultix.Scope{}, false
	}
	return vaultix.Scope{Project: pid, Env: a.cfg.Connect.VaultixEnv, Path: a.cfg.Connect.Path(ws)}, true
}

// storedKeys lists which provider keys Vaultix holds for the workspace (metadata only).
func (a *API) storedKeys(ctx context.Context, tenant, ws string) map[string]bool {
	out := map[string]bool{}
	if s, ok := a.connectScope(ctx, tenant, ws); ok {
		items, _ := a.n2.vx.c.List(ctx, s)
		for _, m := range items {
			out[m.Key] = true
		}
	}
	return out
}

// putConnectToken stores a pasted token in Vaultix, then hands it to a running workspace.
// The value is never logged, echoed back or placed on any command line.
func (a *API) putConnectToken(w http.ResponseWriter, r *http.Request) {
	x, ok := a.connectFor(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Token     string     `json:"token"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil || in.Token == "" {
		auth.Problem(w, http.StatusBadRequest, "Paste a token or API key.")
		return
	}
	s, ok := a.connectScope(r.Context(), x.t.ID, x.ws.ID)
	if !ok {
		auth.Problem(w, http.StatusServiceUnavailable, "Vaultix is unavailable, so the token wasn't saved.")
		return
	}
	if err := a.n2.vx.c.EnsureFolder(r.Context(), s.Project, s.Env, s.Path); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Vaultix is unavailable, so the token wasn't saved.")
		return
	}
	if !a.connectAudit(r, x, "connect.token") {
		refuseUnaudited(w)
		return
	}
	p, _ := a.cfg.Connect.Provider(x.provider)
	if _, err := a.n2.vx.c.Set(r.Context(), s, p.VaultixKey, in.Token, true); err != nil {
		if _, err = a.n2.vx.c.Set(r.Context(), s, p.VaultixKey, in.Token, false); err != nil {
			auth.Problem(w, http.StatusBadGateway, "Vaultix didn't accept the token.")
			return
		}
	}
	if x.ws.State == "running" {
		if err := a.host.PutConnectToken(r.Context(), x.t.ID, x.ws.ID, x.provider, in.Token, in.ExpiresAt); err != nil {
			auth.Problem(w, http.StatusConflict, "Saved in Vaultix, but the workspace rejected it. Check the token and try again.")
			return
		}
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// disconnectProvider signs the CLI out in the workspace and deletes the Vaultix copy.
func (a *API) disconnectProvider(w http.ResponseWriter, r *http.Request) {
	x, ok := a.connectFor(w, r, true)
	if !ok {
		return
	}
	if !a.connectAudit(r, x, "connect.disconnect") {
		refuseUnaudited(w)
		return
	}
	p, _ := a.cfg.Connect.Provider(x.provider)
	if s, ok := a.connectScope(r.Context(), x.t.ID, x.ws.ID); ok {
		_ = a.n2.vx.c.Delete(r.Context(), s, p.VaultixKey)
	}
	if x.ws.State == "running" {
		if err := a.host.DisconnectProvider(r.Context(), x.t.ID, x.ws.ID, x.provider); err != nil {
			auth.Problem(w, http.StatusConflict, "The workspace didn't sign out. Try again once it's running.")
			return
		}
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// pushConnectionsAfterStart re-injects every Vaultix-held token into a freshly started workspace.
func (a *API) pushConnectionsAfterStart(ctx context.Context, tenant string, ws db.Workspace, act string) {
	if !a.cfg.Connect.Enabled || (act != "start" && act != "restart") {
		return
	}
	s, ok := a.connectScope(ctx, tenant, ws.ID)
	if !ok {
		return
	}
	for _, p := range a.cfg.Connect.Providers {
		_, val, err := a.n2.vx.c.Reveal(ctx, s, p.VaultixKey)
		if err != nil || val == "" {
			continue
		}
		if err := a.host.PutConnectToken(ctx, tenant, ws.ID, p.ID, val, nil); err != nil {
			log.Printf("connect: %s after %s %s not injected", p.ID, act, ws.ID)
		}
	}
}

// ideTerminalURL deep-links the workspace's openvscode-server, through the edge sign-in, at
// the configured folder with the terminal panel requested (see deploy/image/shippin-ide).
func (a *API) ideTerminalURL(wsURL string) string {
	if wsURL == "" {
		return ""
	}
	target := wsURL + "/?folder=" + url.QueryEscape(a.cfg.Connect.IDEFolder) + "&shippin=terminal"
	return "/auth/ws-login?rd=" + url.QueryEscape(target)
}
