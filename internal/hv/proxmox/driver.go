// Package proxmox is the Proxmox VE driver.
//
// It speaks /api2/json, then reads the guest back. TLS is always verified.
// There is no skip flag.
package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
)

// Endpoint is one Proxmox API. Token is resolved per call and is not stored here.
type Endpoint struct {
	BaseURL string // https://host:8006, no path
	Node    string
	TokenID string
	CAPEM   []byte // extra trust anchors; system roots are always included
	// TrustedPEM is one certificate an operator accepted. It replaces the system roots.
	TrustedPEM []byte
}

// Client is a Proxmox driver.
type Client struct {
	Endpoint Endpoint
	Secret   func() (string, error)
	HTTP     *http.Client
	Now      func() time.Time
	// PollBase and PollMax bound UPID polling. Zero uses 200ms and 5s.
	PollBase time.Duration
	PollMax  time.Duration
}

// New builds a client. HTTP is set when CAPEM or the default transport must verify.
func New(ep Endpoint, secret func() (string, error)) (*Client, error) {
	if ep.BaseURL == "" || ep.TokenID == "" || secret == nil {
		return nil, fmt.Errorf("proxmox: base URL, token id, and secret resolver are required")
	}
	tlsCfg, err := hv.TLSConfig(ep.CAPEM, ep.TrustedPEM, nil)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{TLSClientConfig: tlsCfg}
	return &Client{
		Endpoint: ep,
		Secret:   secret,
		HTTP:     &http.Client{Timeout: 15 * time.Second, Transport: tr},
		Now:      time.Now,
	}, nil
}

// nodeOf is where a guest actually is. In a cluster that may not be the node attached to.
func (c *Client) nodeOf(g hv.Guest) string {
	if g.Node != "" {
		return g.Node
	}
	return c.Endpoint.Node
}

func (c *Client) Kind() string { return "proxmox" }

func (c *Client) Health(ctx context.Context) (hv.Health, error) {
	h := hv.Health{Checked: c.now()}
	body, code, err := c.do(ctx, http.MethodGet, "/version", nil)
	if err != nil {
		h.Error = err.Error()
		return h, err
	}
	if code != http.StatusOK {
		h.Error = fmt.Sprintf("version HTTP %d: %s", code, clip(body))
		return h, fmt.Errorf("proxmox: %s", h.Error)
	}
	var ver struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &ver)
	h.Online = true
	h.Version = ver.Data.Version
	return h, nil
}

func (c *Client) Guests(ctx context.Context) ([]hv.Guest, error) {
	body, code, err := c.do(ctx, http.MethodGet, "/cluster/resources?type=vm", nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: resources HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []resource `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("proxmox: resources: %w", err)
	}
	out := make([]hv.Guest, 0, len(r.Data))
	for _, item := range r.Data {
		out = append(out, item.guest())
	}
	return out, nil
}

// Guest reads one guest and its status right now. /cluster/resources only catches up when pvestatd
// next runs, about every ten seconds, so just after a change it still reports the old status.
func (c *Client) Guest(ctx context.Context, id string) (hv.Guest, error) {
	all, err := c.Guests(ctx)
	if err != nil {
		return hv.Guest{}, err
	}
	for _, g := range all {
		if g.ID != id {
			continue
		}
		kind, err := apiKind(g)
		if err != nil {
			return hv.Guest{}, err
		}
		body, code, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/%s/%s/status/current", c.nodeOf(g), kind, id), nil)
		if err != nil {
			return hv.Guest{}, err
		}
		if code != http.StatusOK {
			return hv.Guest{}, fmt.Errorf("proxmox: status HTTP %d: %s", code, clip(body))
		}
		var r struct {
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return hv.Guest{}, fmt.Errorf("proxmox: status: %w", err)
		}
		if r.Data.Status != "" {
			g.Status = r.Data.Status
		}
		return g, nil
	}
	return hv.Guest{}, fmt.Errorf("proxmox: guest %s: %w", id, hv.ErrGuestNotFound)
}

func apiKind(g hv.Guest) (string, error) {
	switch g.Kind {
	case hv.KindVM:
		return "qemu", nil
	case hv.KindCT:
		return "lxc", nil
	default:
		return "", fmt.Errorf("proxmox: guest %s has unknown kind %q", g.ID, g.Kind)
	}
}

func (c *Client) Storage(ctx context.Context) ([]hv.Volume, error) {
	path := fmt.Sprintf("/nodes/%s/storage", c.Endpoint.Node)
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: storage HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []struct {
			Storage string `json:"storage"`
			Type    string `json:"type"`
			Content string `json:"content"`
			Used    int64  `json:"used"`
			Total   int64  `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]hv.Volume, 0, len(r.Data))
	for _, s := range r.Data {
		out = append(out, hv.Volume{Name: s.Storage, Type: s.Type, Node: c.Endpoint.Node, Content: s.Content, Used: s.Used, Total: s.Total})
	}
	return out, nil
}

func (c *Client) Networks(ctx context.Context) ([]hv.Network, error) {
	path := fmt.Sprintf("/nodes/%s/network", c.Endpoint.Node)
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: network HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []struct {
			Iface string `json:"iface"`
			Type  string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]hv.Network, 0, len(r.Data))
	for _, n := range r.Data {
		out = append(out, hv.Network{Name: n.Iface, Type: n.Type, Node: c.Endpoint.Node})
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, form url.Values) ([]byte, int, error) {
	secret, err := c.Secret()
	if err != nil {
		return nil, 0, err
	}
	if secret == "" {
		return nil, 0, fmt.Errorf("proxmox: token secret is empty")
	}
	u := strings.TrimRight(c.Endpoint.BaseURL, "/") + "/api2/json" + path
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, 0, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.Endpoint.TokenID+"="+secret)
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

type resource struct {
	VMID       int     `json:"vmid"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	Node       string  `json:"node"`
	Pool       string  `json:"pool"`
	Tags       string  `json:"tags"`
	Template   int     `json:"template"`
	Uptime     int64   `json:"uptime"`
	CPU        float64 `json:"cpu"`
	MaxCPU     int     `json:"maxcpu"`
	Memory     int64   `json:"mem"`
	MaxMemory  int64   `json:"maxmem"`
	Disk       int64   `json:"disk"`
	MaxDisk    int64   `json:"maxdisk"`
	NetworkIn  int64   `json:"netin"`
	NetworkOut int64   `json:"netout"`
}

func (r resource) guest() hv.Guest {
	kind := hv.KindVM
	if r.Type == "lxc" {
		kind = hv.KindCT
	}
	return hv.Guest{
		ID: strconv.Itoa(r.VMID), Name: r.Name, Kind: kind, Status: r.Status, Node: r.Node, Tags: r.Tags,
		Pool: r.Pool, Template: r.Template == 1, Uptime: r.Uptime,
		Metrics: &hv.GuestMetrics{CPU: r.CPU, CPUCount: r.MaxCPU, Memory: r.Memory, MaxMemory: r.MaxMemory,
			Disk: r.Disk, MaxDisk: r.MaxDisk, NetworkIn: r.NetworkIn, NetworkOut: r.NetworkOut},
	}
}

func validSnap(s string) bool {
	if len(s) < 2 || len(s) > 40 {
		return false
	}
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, c := range s[1:] {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func clip(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
