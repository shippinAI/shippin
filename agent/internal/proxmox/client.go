// Package proxmox reads Proxmox VE observations and submits lifecycle actions. It does not authorize workloads.
package proxmox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const maxResponseBytes = 8 << 20

type Instance struct {
	ID     int    `json:"vmid"`
	Name   string `json:"name"`
	Node   string `json:"node"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// Observation is provider data, not a Shippin workload or readiness decision.
type Observation struct {
	Provider      string     `json:"provider"`
	ServerVersion string     `json:"server_version"`
	Instances     []Instance `json:"instances"`
}

type Client struct {
	base, authorization string
	http                *http.Client
}

// New connects to an explicit HTTPS origin. caPEM optionally adds an operator's
// CA to system trust; certificate and hostname verification are always required.
func New(origin, tokenID, tokenSecret string, caPEM []byte) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.TrimSpace(origin) != origin {
		return nil, errors.New("proxmox endpoint must be an HTTPS origin without credentials, path, query or fragment")
	}
	if !validTokenPart(tokenID) || !validTokenPart(tokenSecret) || strings.Contains(tokenID, "=") {
		return nil, errors.New("proxmox API token ID and secret must be supplied without whitespace or control characters")
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if len(caPEM) > 0 && !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("invalid proxmox CA certificate")
	}
	return &Client{
		base: strings.TrimSuffix(u.String(), "/"), authorization: "PVEAPIToken=" + tokenID + "=" + tokenSecret,
		http: &http.Client{
			Timeout:       15 * time.Second,
			Transport:     &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func validTokenPart(value string) bool {
	return value != "" && strings.IndexFunc(value, func(r rune) bool { return r <= ' ' || r == 127 }) < 0
}

// Inspect reads a fresh cluster inventory; any failed or incomplete response
// returns no observation. Visibility is limited by the supplied token's ACLs.
func (c *Client) Inspect(ctx context.Context) (Observation, error) {
	var version struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, "/api2/json/version", &version); err != nil {
		return Observation{}, fmt.Errorf("read proxmox version: %w", err)
	}
	if version.Version == "" {
		return Observation{}, errors.New("proxmox response has no version")
	}
	var instances []Instance
	if err := c.get(ctx, "/api2/json/cluster/resources?type=vm", &instances); err != nil {
		return Observation{}, fmt.Errorf("read proxmox inventory: %w", err)
	}
	if instances == nil {
		return Observation{}, errors.New("proxmox inventory is not a list")
	}
	seen := map[int]bool{}
	for _, item := range instances {
		if item.ID <= 0 || item.Node == "" || (item.Type != "qemu" && item.Type != "lxc") || item.Status == "" || seen[item.ID] {
			return Observation{}, errors.New("proxmox inventory contains incomplete or duplicate instances")
		}
		seen[item.ID] = true
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	return Observation{Provider: "proxmox", ServerVersion: version.Version, Instances: instances}, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return errors.New("invalid proxmox request")
	}
	req.Header.Set("Authorization", c.authorization)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("proxmox connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("proxmox returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return errors.New("proxmox response unreadable or too large")
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" || json.Unmarshal(envelope.Data, out) != nil {
		return errors.New("invalid proxmox response")
	}
	return nil
}
