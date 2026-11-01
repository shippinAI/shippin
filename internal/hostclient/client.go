// Package hostclient talks to shippin-hostd through the container's proxy device.
package hostclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	Base string
	HTTP *http.Client
}

func New(base string) *Client {
	return &Client{Base: base, HTTP: &http.Client{Timeout: 6 * time.Minute}}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body bytes.Buffer
	if in != nil {
		_ = json.NewEncoder(&body).Encode(in)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.NewDecoder(res.Body).Decode(&e)
		return &StatusError{Code: res.StatusCode, Msg: e.Error}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *Client) PlatformInstances(ctx context.Context) ([]Instance, error) {
	var env struct{ Metadata []Instance }
	err := c.do(ctx, http.MethodGet, "/platform/instances", nil, &env)
	return env.Metadata, err
}

func (c *Client) TenantInstances(ctx context.Context, t string) ([]Instance, error) {
	list, _, err := c.TenantInventory(ctx, t)
	return list, err
}

// TenantInventory is TenantInstances plus whether hostd could read only part of it (tenant VM down).
func (c *Client) TenantInventory(ctx context.Context, t string) ([]Instance, bool, error) {
	var env struct {
		Metadata []Instance
		Partial  *bool
	}
	err := c.do(ctx, http.MethodGet, "/tenants/"+t+"/instances", nil, &env)
	if err != nil {
		return nil, true, err
	}
	if env.Metadata == nil || env.Partial == nil {
		return nil, true, fmt.Errorf("incomplete inventory envelope")
	}
	seen := map[string]bool{}
	for _, i := range env.Metadata {
		if i.Name == "" || seen[i.Name] {
			return nil, true, fmt.Errorf("ambiguous inventory entry")
		}
		seen[i.Name] = true
	}
	return env.Metadata, *env.Partial, nil
}

type Provisioned struct {
	SSHIP string `json:"ssh_ip"`
}

func (c *Client) Provision(ctx context.Context, t, plan string) (*Provisioned, error) {
	out := &Provisioned{}
	return out, c.do(ctx, http.MethodPost, "/tenants/"+t+"/provision", map[string]string{"plan": plan}, out)
}

type WorkspaceReq struct {
	Name     string   `json:"name"`
	Plan     string   `json:"plan"`
	Keys     []string `json:"ssh_public_keys"`
	Port     int      `json:"ssh_port"`
	SSHIP    string   `json:"ssh_ip"`
	Creator  string   `json:"created_by"`
	Host     string   `json:"host"`
	Domains  []string `json:"domains"`
	Rules    any      `json:"firewall"`
	Image    string   `json:"image"`
	Size     any      `json:"size"`
	Repo     string   `json:"repo"`
	Startup  string   `json:"startup"`
	Previews any      `json:"previews"`
	Project  string   `json:"project_id,omitempty"` // M1: inner project pj-<uuid> on the boundary
	// Environment and Boundary become user.shippin.* labels on the instance (reconciliation).
	Environment string `json:"environment_id,omitempty"`
	Boundary    string `json:"boundary_id,omitempty"`
	// AIHome is the creator's account id when they have an AI home in this tenant (M2).
	AIHome string `json:"ai_home_account,omitempty"`
	// GitToken is set only on creates of GitHub-imported workspaces (M3).
	GitToken *GitToken `json:"git_token,omitempty"`
}

func (c *Client) CreateWorkspace(ctx context.Context, t, w string, r WorkspaceReq) (string, error) {
	var out struct{ IPv4 string }
	err := c.do(ctx, http.MethodPost, "/tenants/"+t+"/workspaces/"+w, r, &out)
	return out.IPv4, err
}

func (c *Client) Action(ctx context.Context, t, w, action string, r WorkspaceReq) error {
	return c.do(ctx, http.MethodPost, "/tenants/"+t+"/workspaces/"+w+"/"+action, r, nil)
}

type Plan struct {
	CPU    int    `json:"workspace_cpu"`
	Memory string `json:"workspace_memory"`
	Home   int    `json:"home_gib"`
	Root   int    `json:"root_gib"`
}

func (c *Client) Plan(ctx context.Context, name string) (*Plan, error) {
	p := &Plan{}
	return p, c.do(ctx, http.MethodGet, "/plans/"+name, nil, p)
}

// StarterDB is the tenant's starter Postgres (no password; hostd keeps it in sops).
type StarterDB struct {
	Database string   `json:"database"`
	Role     string   `json:"role"`
	Addr     string   `json:"addr"`
	Tables   []string `json:"tables"`
}

// EnsureStarterDB creates the tenant's starter database if missing (idempotent).
func (c *Client) EnsureStarterDB(ctx context.Context, t string) (*StarterDB, error) {
	out := &StarterDB{}
	return out, c.do(ctx, http.MethodPost, "/tenants/"+t+"/starter-db", nil, out)
}
