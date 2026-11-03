package hostclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// AgentStatus is a workspace's report on its build agent (booleans, a timestamp, a count).
type AgentStatus struct {
	Session      bool  `json:"session"`
	Activity     int64 `json:"activity"`
	Exited       bool  `json:"exited"`
	ExitCode     int   `json:"exit_code"`
	FilesChanged int   `json:"files_changed"`
}

// PutPrompt writes the prompt into the workspace (hostd sends it on stdin, never on a command line).
func (c *Client) PutPrompt(ctx context.Context, t, w, prompt string) error {
	return c.do(ctx, http.MethodPut, "/tenants/"+t+"/workspaces/"+w+"/prompt", map[string]string{"prompt": prompt}, nil)
}

// StartAgent starts the user's own CLI (provider id) on the prompt, or resumes its last conversation.
func (c *Client) StartAgent(ctx context.Context, t, w, provider string, resume bool) (bool, error) {
	var out struct{ Started bool }
	err := c.do(ctx, http.MethodPost, "/tenants/"+t+"/workspaces/"+w+"/agent", map[string]any{"provider": provider, "resume": resume}, &out)
	return out.Started, err
}

// AgentStatus reads the build agent's state in a running workspace.
func (c *Client) AgentStatus(ctx context.Context, t, w string) (*AgentStatus, error) {
	out := &AgentStatus{}
	return out, c.do(ctx, http.MethodGet, "/tenants/"+t+"/workspaces/"+w+"/agent", nil, out)
}

// ServiceEndpoint is where a service instance answers inside the boundary VM (internal only).
type ServiceEndpoint struct {
	IPv4 string `json:"ipv4"`
	Port int    `json:"port"`
	URL  string `json:"url"`
}

func svcPath(t, pj, si string) string { return "/tenants/" + t + "/projects/" + pj + "/services/" + si }

// Svc addresses one service instance: tenant, project (the namespace inside a shared instance)
// and instance id. Shared is the tenant VM's one rrflow instance (Notion D36).
type Svc struct {
	Tenant, Project, ID string
	Shared              bool
}

func (s Svc) path(sub string) string {
	p := svcPath(s.Tenant, s.Project, s.ID) + sub
	if s.Shared {
		if strings.Contains(p, "?") {
			return p + "&scope=vm"
		}
		return p + "?scope=vm"
	}
	return p
}

// EnsureRRFlow creates or repairs the rrflow/state instance.
func (c *Client) EnsureRRFlow(ctx context.Context, s Svc, plan string) (*ServiceEndpoint, error) {
	out := &ServiceEndpoint{}
	return out, c.do(ctx, http.MethodPost, s.path("/rrflow"), map[string]any{"plan": plan, "shared": s.Shared}, out)
}

// BindService binds the instance to a workspace (hostd mints the token; it never reaches us).
func (c *Client) BindService(ctx context.Context, s Svc, sb, ws string) error {
	return c.do(ctx, http.MethodPost, s.path("/bindings/"+sb), map[string]any{"workspace_id": ws, "shared": s.Shared}, nil)
}

// UnbindService revokes a binding.
func (c *Client) UnbindService(ctx context.Context, s Svc, sb, ws string) error {
	return c.do(ctx, http.MethodDelete, s.path("/bindings/"+sb+"?workspace_id="+url.QueryEscape(ws)), nil, nil)
}

// ServiceState exports one rrflow/state kind (sessions|memory|decisions) as raw JSON.
func (c *Client) ServiceState(ctx context.Context, s Svc, kind string, limit int) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodGet, s.path("/state/"+kind+"?limit="+strconv.Itoa(limit)), nil, &out)
}

// MigrateService moves pre-D36 per-project instance s into namespace s.Project of shared
// instance to, then deletes s (hostd MigrateRRFlow).
func (c *Client) MigrateService(ctx context.Context, s Svc, to string) error {
	return c.do(ctx, http.MethodPost, svcPath(s.Tenant, s.Project, s.ID)+"/migrate-to/"+to, map[string]string{}, nil)
}
