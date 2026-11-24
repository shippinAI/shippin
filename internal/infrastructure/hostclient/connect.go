package hostclient

import (
	"context"
	"net/http"
	"time"
)

// ConnectStatus is the workspace credential writer's report: no credential fields.
type ConnectStatus struct {
	CheckedAt time.Time `json:"checked_at"`
	Providers []struct {
		Provider  string     `json:"provider"`
		Connected bool       `json:"connected"`
		Method    string     `json:"method,omitempty"`
		ExpiresAt *time.Time `json:"expires_at,omitempty"`
	} `json:"providers"`
}

// LoginState is a browser sign-in in progress: the verification URL and one-time code only.
type LoginState struct {
	Provider  string    `json:"provider"`
	State     string    `json:"state"`
	URL       string    `json:"verification_url,omitempty"`
	Code      string    `json:"user_code,omitempty"`
	CodeInput bool      `json:"code_input"`
	StartedAt time.Time `json:"started_at"`
}

func connectPath(t, w string) string { return "/tenants/" + t + "/workspaces/" + w + "/connect" }

func (c *Client) ConnectStatus(ctx context.Context, t, w string) (*ConnectStatus, error) {
	out := &ConnectStatus{}
	return out, c.do(ctx, http.MethodGet, connectPath(t, w), nil, out)
}

func (c *Client) StartLogin(ctx context.Context, t, w, p string) (*LoginState, error) {
	out := &LoginState{}
	return out, c.do(ctx, http.MethodPost, connectPath(t, w)+"/"+p+"/login", nil, out)
}

func (c *Client) Login(ctx context.Context, t, w, p string) (*LoginState, error) {
	out := &LoginState{}
	return out, c.do(ctx, http.MethodGet, connectPath(t, w)+"/"+p+"/login", nil, out)
}

// SubmitLoginCode types a code the vendor showed back into the waiting CLI (body, never argv).
func (c *Client) SubmitLoginCode(ctx context.Context, t, w, p, code string) error {
	return c.do(ctx, http.MethodPost, connectPath(t, w)+"/"+p+"/code", map[string]string{"code": code}, nil)
}

// PutConnectToken hands a token to the workspace's credential writer (JSON body only).
func (c *Client) PutConnectToken(ctx context.Context, t, w, p, token string, expires *time.Time) error {
	in := map[string]any{"token": token}
	if expires != nil {
		in["expires_at"] = expires
	}
	return c.do(ctx, http.MethodPut, connectPath(t, w)+"/"+p+"/token", in, nil)
}

func (c *Client) DisconnectProvider(ctx context.Context, t, w, p string) error {
	return c.do(ctx, http.MethodDelete, connectPath(t, w)+"/"+p, nil, nil)
}
