// Package incus reads local Incus observations. It does not authorize workloads.
package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxResponseBytes = 8 << 20

// Instance is a provider observation, not a Shippin workload or tenant record.
type Instance struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// Observation describes one explicitly selected Incus project.
type Observation struct {
	Provider      string     `json:"provider"`
	Project       string     `json:"project"`
	ServerVersion string     `json:"server_version"`
	Instances     []Instance `json:"instances"`
}

type Client struct{ http *http.Client }

// NewLocal connects only to the supplied local socket; it never opens a TCP connection.
func NewLocal(socket string) (*Client, error) {
	if !filepath.IsAbs(socket) || strings.TrimSpace(socket) != socket {
		return nil, errors.New("incus socket must be an absolute path")
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}
	return &Client{http: &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

// Inspect obtains a fresh, read-only snapshot. Project must be explicitly selected.
// A failed request yields no partial observation.
func (c *Client) Inspect(ctx context.Context, project string) (Observation, error) {
	if project == "" || strings.TrimSpace(project) != project {
		return Observation{}, errors.New("incus project must be explicitly specified")
	}
	var server struct {
		Environment struct {
			ServerVersion string `json:"server_version"`
		} `json:"environment"`
	}
	if err := c.get(ctx, "/1.0", &server); err != nil {
		return Observation{}, fmt.Errorf("read incus server: %w", err)
	}
	if server.Environment.ServerVersion == "" {
		return Observation{}, errors.New("incus server response has no version")
	}
	query := url.Values{"project": {project}, "recursion": {"1"}}
	var instances []Instance
	if err := c.get(ctx, "/1.0/instances?"+query.Encode(), &instances); err != nil {
		return Observation{}, fmt.Errorf("read incus instances: %w", err)
	}
	if instances == nil {
		return Observation{}, errors.New("incus instances response is not a list")
	}
	for _, instance := range instances {
		if instance.Name == "" || instance.Type == "" || instance.Status == "" {
			return Observation{}, errors.New("incus instance response is incomplete")
		}
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].Name < instances[j].Name })
	return Observation{Provider: "incus", Project: project, ServerVersion: server.Environment.ServerVersion, Instances: instances}, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://incus.local"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("local socket request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("incus returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return errors.New("cannot read incus response")
	}
	if len(data) > maxResponseBytes {
		return errors.New("incus response exceeds size limit")
	}
	var envelope struct {
		Type       string          `json:"type"`
		StatusCode int             `json:"status_code"`
		Metadata   json.RawMessage `json:"metadata"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Type != "sync" || envelope.StatusCode != 200 || len(envelope.Metadata) == 0 {
		return errors.New("invalid incus response")
	}
	if err := json.Unmarshal(envelope.Metadata, out); err != nil {
		return errors.New("invalid incus response metadata")
	}
	return nil
}
