package hostclient

import (
	"context"
	"net/http"
	"time"
)

// AIStatus is hostd's typed copy of a container's exit-code-only sign-in report. It has no
// credential fields: the console can only learn "signed in or not" per provider.
type AIStatus struct {
	Version   int       `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
	Providers []struct {
		Provider  string `json:"provider"`
		Installed bool   `json:"installed"`
		SignedIn  bool   `json:"signed_in"`
		AuthKnown bool   `json:"auth_known"`
	} `json:"providers"`
}

// AttachAIHome mounts the account's own AI home into one of their existing workspaces.
func (c *Client) AttachAIHome(ctx context.Context, t, w, account string) error {
	return c.do(ctx, http.MethodPost, "/tenants/"+t+"/workspaces/"+w+"/ai-home", map[string]string{"account": account}, nil)
}

// StartUtility starts the account's activation terminal on host; it returns the container address.
func (c *Client) StartUtility(ctx context.Context, t, account, plan, host string) (string, error) {
	var out struct{ IPv4 string }
	err := c.do(ctx, http.MethodPost, "/tenants/"+t+"/utilities/"+account+"/start",
		map[string]string{"plan": plan, "host": host}, &out)
	return out.IPv4, err
}

// StopUtility unpublishes and stops the account's activation terminal.
func (c *Client) StopUtility(ctx context.Context, t, account string) error {
	return c.do(ctx, http.MethodPost, "/tenants/"+t+"/utilities/"+account+"/stop", nil, nil)
}

// UtilityLogout runs the vendor's own sign-out for provider in the account's terminal container.
func (c *Client) UtilityLogout(ctx context.Context, t, account, provider string) error {
	return c.do(ctx, http.MethodPost, "/tenants/"+t+"/utilities/"+account+"/logout", map[string]string{"provider": provider}, nil)
}

// UtilityAIStatus reads the sign-in report inside the account's terminal container.
func (c *Client) UtilityAIStatus(ctx context.Context, t, account string) (*AIStatus, error) {
	out := &AIStatus{}
	return out, c.do(ctx, http.MethodGet, "/tenants/"+t+"/utilities/"+account+"/ai-status", nil, out)
}

// WorkspaceAIStatus reads the sign-in report inside a workspace carrying the user's AI home.
func (c *Client) WorkspaceAIStatus(ctx context.Context, t, w string) (*AIStatus, error) {
	out := &AIStatus{}
	return out, c.do(ctx, http.MethodGet, "/tenants/"+t+"/workspaces/"+w+"/ai-status", nil, out)
}

// GitToken is a workspace's 1-hour GitHub installation token (M3), narrowed to its repository.
type GitToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PutGitToken refreshes the token file of a running imported workspace.
func (c *Client) PutGitToken(ctx context.Context, t, w string, tok GitToken) error {
	return c.do(ctx, http.MethodPut, "/tenants/"+t+"/workspaces/"+w+"/git-token", tok, nil)
}
