package hostclient

import (
	"context"
	"encoding/json"
	"net/http"
)

// Raw passes read-only hostd operator views through unchanged.
func (c *Client) Raw(ctx context.Context, path string) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodGet, path, nil, &out)
}

// TenantNetworks maps Incus project name → {network, subnet, router}.
func (c *Client) TenantNetworks(ctx context.Context) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	return out, c.do(ctx, http.MethodGet, "/networks/tenants", nil, &out)
}

// DeleteTenant tears down a tenant's Incus silo; the encrypted dataset is kept.
func (c *Client) DeleteTenant(ctx context.Context, t string) (string, error) {
	var out struct {
		Dataset string `json:"dataset_kept"`
	}
	return out.Dataset, c.do(ctx, http.MethodDelete, "/tenants/"+t, nil, &out)
}

// Progress is the current provisioning step reported by hostd ("" when idle).
func (c *Client) Progress(ctx context.Context, t, w string) string {
	var out struct{ Progress string }
	_ = c.do(ctx, http.MethodGet, "/tenants/"+t+"/workspaces/"+w+"/progress", nil, &out)
	return out.Progress
}

// ListenPort is a TCP port a workspace listens on (local_only = bound to loopback).
type ListenPort struct {
	Port      int  `json:"port"`
	LocalOnly bool `json:"local_only"`
}

func (c *Client) Listening(ctx context.Context, t, w string) ([]ListenPort, error) {
	var out struct{ Items []ListenPort }
	return out.Items, c.do(ctx, http.MethodGet, "/tenants/"+t+"/workspaces/"+w+"/listening", nil, &out)
}

// Found is a suggested project folder in a devspace (hostd discover; names and manifests only).
type Found struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Git       bool     `json:"git"`
	Remote    string   `json:"remote,omitempty"`
	Manifests []string `json:"manifests"`
}

// Discover lists project-like folders in the devspace home (read-only scan).
func (c *Client) Discover(ctx context.Context, t, w string) ([]Found, error) {
	var out struct{ Items []Found }
	return out.Items, c.do(ctx, http.MethodGet, "/tenants/"+t+"/workspaces/"+w+"/discover", nil, &out)
}

// GitOp is an allowlisted git operation in a devspace ("clone" or "publish"; see hostd.GitOp).
type GitOp struct {
	Op       string    `json:"op"`
	Path     string    `json:"path"`
	Repo     string    `json:"repo"`
	Branch   string    `json:"branch"`
	GitToken *GitToken `json:"git_token"`
}

// Git runs op in the devspace w (the token is installed for the image's credential helper first).
func (c *Client) Git(ctx context.Context, t, w string, op GitOp) error {
	return c.do(ctx, http.MethodPost, "/tenants/"+t+"/workspaces/"+w+"/git", op, nil)
}
