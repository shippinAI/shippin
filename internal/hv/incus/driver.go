// Package incus is the Incus driver. It speaks the REST API hostd already uses
// through the incus CLI, and it does not shell out.
package incus

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
)

// Endpoint is one Incus API.
type Endpoint struct {
	BaseURL    string // https://host:8443
	Project    string // "" is the server's default project
	CAPEM      []byte
	TrustedPEM []byte           // one certificate an operator accepted; it replaces the system roots
	ClientCert *tls.Certificate // Incus trusts remotes by certificate; nil uses the bearer secret alone
}

// Client is an Incus driver.
type Client struct {
	Endpoint Endpoint
	Secret   func() (string, error) // bearer; may return "" when the test server does not check it
	HTTP     *http.Client
	Now      func() time.Time
}

// New builds a verifying client.
func New(ep Endpoint, secret func() (string, error)) (*Client, error) {
	if ep.BaseURL == "" || secret == nil {
		return nil, fmt.Errorf("incus: base URL and secret resolver are required")
	}
	tlsCfg, err := hv.TLSConfig(ep.CAPEM, ep.TrustedPEM, ep.ClientCert)
	if err != nil {
		return nil, err
	}
	return &Client{
		Endpoint: ep,
		Secret:   secret,
		// Longer than operationWait: a mutation holds this request open until Incus finishes.
		HTTP: &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsCfg}},
		Now:  time.Now,
	}, nil
}

func (c *Client) Kind() string { return "incus" }

// operationWait is how long Incus may hold a mutation open, in seconds.
const operationWait = "45"

// waitAgain keeps a server that answers a wait at once from being asked in a tight loop.
var waitAgain = time.Second

// path adds the configured project, so a call never lands in another project by default.
func (c *Client) path(p string, params ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(params); i += 2 {
		q.Set(params[i], params[i+1])
	}
	if c.Endpoint.Project != "" {
		q.Set("project", c.Endpoint.Project)
	}
	if len(q) == 0 {
		return p
	}
	return p + "?" + q.Encode()
}

func instancePath(id string) string { return "/1.0/instances/" + url.PathEscape(id) }

// operationReply is an answer about an operation. Metadata is empty when Type is "error".
type operationReply struct {
	Type     string `json:"type"`
	Error    string `json:"error"`
	Metadata struct {
		StatusCode int    `json:"status_code"`
		Err        string `json:"err"`
	} `json:"metadata"`
}

// wait blocks until the async operation in body ends. Reading a guest before this returns the
// old state. A response that names no operation has nothing to wait for. Incus lets go after
// operationWait even if the work is still going, as a long image download is, so this asks again until
// it has ended or ctx has.
func (c *Client) wait(ctx context.Context, body []byte) error {
	var started struct {
		Type     string `json:"type"`
		Metadata struct {
			ID string `json:"id"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &started) != nil || started.Type != "async" || started.Metadata.ID == "" {
		return nil
	}
	op := "/1.0/operations/" + url.PathEscape(started.Metadata.ID)
	for {
		raw, code, err := c.do(ctx, http.MethodGet, c.path(op+"/wait", "timeout", operationWait), nil)
		if err != nil {
			return err
		}
		if code != http.StatusOK {
			return fmt.Errorf("incus: operation HTTP %d: %s", code, clip(raw))
		}
		var done operationReply
		if err := json.Unmarshal(raw, &done); err != nil {
			return err
		}
		// A wait that ran out and work that failed get the same answer here: HTTP 200, type "error", no
		// metadata. Only reading the operation tells them apart, and Incus forgets a failed one within seconds.
		if done.Type == "error" {
			reason := done.Error
			raw, code, err = c.do(ctx, http.MethodGet, c.path(op), nil)
			if err != nil {
				return err
			}
			done = operationReply{}
			if code != http.StatusOK || json.Unmarshal(raw, &done) != nil || done.Type == "error" {
				return fmt.Errorf("incus: %s", reason)
			}
		}
		status := done.Metadata.StatusCode
		if status == http.StatusOK {
			return nil
		}
		if !stillGoing(status) {
			return fmt.Errorf("incus: operation did not succeed: status %d: %s", status, done.Metadata.Err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitAgain):
		}
	}
}

// stillGoing is an Incus status for work that has not ended: created, started, running, cancelling,
// pending, starting, stopping or aborting.
func stillGoing(status int) bool {
	switch status {
	case 100, 101, 103, 104, 105, 106, 107, 108:
		return true
	}
	return false
}

func (c *Client) Health(ctx context.Context) (hv.Health, error) {
	h := hv.Health{Checked: c.now()}
	body, code, err := c.do(ctx, http.MethodGet, "/1.0", nil)
	if err != nil {
		h.Error = err.Error()
		return h, err
	}
	if code != http.StatusOK {
		h.Error = fmt.Sprintf("HTTP %d: %s", code, clip(body))
		return h, fmt.Errorf("incus: %s", h.Error)
	}
	var api struct {
		Metadata struct {
			Auth        string `json:"auth"`
			Environment struct {
				ServerVersion string `json:"server_version"`
			} `json:"environment"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(body, &api)
	// An untrusted client still gets 200 here, so a plain answer does not prove the panel can manage it.
	if api.Metadata.Auth == "untrusted" {
		h.Error = "the server no longer trusts this panel; detach this host, then add it again with a new trust token"
		return h, fmt.Errorf("incus: %s", h.Error)
	}
	h.Online = true
	h.Version = api.Metadata.Environment.ServerVersion
	return h, nil
}

func (c *Client) Guests(ctx context.Context) ([]hv.Guest, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path("/1.0/instances", "recursion", "1"), nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("incus: instances HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata []instance `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]hv.Guest, 0, len(r.Metadata))
	for _, in := range r.Metadata {
		out = append(out, in.guest())
	}
	return out, nil
}

func (c *Client) Guest(ctx context.Context, id string) (hv.Guest, error) {
	instance, err := c.instanceDetails(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	return instance.guest(), nil
}

func (c *Client) instanceDetails(ctx context.Context, id string) (instance, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path(instancePath(id)), nil)
	if err != nil {
		return instance{}, err
	}
	if code == http.StatusNotFound {
		return instance{}, fmt.Errorf("incus: guest %s: %w", id, hv.ErrGuestNotFound)
	}
	if code != http.StatusOK {
		return instance{}, fmt.Errorf("incus: guest HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata instance `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return instance{}, err
	}
	return r.Metadata, nil
}

func (c *Client) Storage(ctx context.Context) ([]hv.Volume, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path("/1.0/storage-pools", "recursion", "1"), nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("incus: storage HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata []struct {
			Name   string `json:"name"`
			Driver string `json:"driver"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]hv.Volume, 0, len(r.Metadata))
	for _, p := range r.Metadata {
		out = append(out, hv.Volume{Name: p.Name, Type: p.Driver})
	}
	return out, nil
}

func (c *Client) Networks(ctx context.Context) ([]hv.Network, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path("/1.0/networks", "recursion", "1"), nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("incus: networks HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]hv.Network, 0, len(r.Metadata))
	for _, n := range r.Metadata {
		out = append(out, hv.Network{Name: n.Name, Type: n.Type})
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, raw []byte) ([]byte, int, error) {
	secret, err := c.Secret()
	if err != nil {
		return nil, 0, err
	}
	u := strings.TrimRight(c.Endpoint.BaseURL, "/") + path
	var body io.Reader
	if raw != nil {
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, 0, err
	}
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return b, res.StatusCode, err
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

type instance struct {
	Name   string            `json:"name"`
	Status string            `json:"status"`
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
}

func (in instance) guest() hv.Guest {
	kind := hv.KindCT
	if in.Type == "virtual-machine" {
		kind = hv.KindVM
	}
	return hv.Guest{ID: in.Name, Name: in.Name, Kind: kind, Status: in.Status}
}

func clip(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
