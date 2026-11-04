// Package networking owns configured network integrations and their observed inventory.
package networking

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const (
	CloudflareTunnel = "cloudflare_tunnel"
	Tailscale        = "tailscale"
	Custom           = "custom"
)

var ErrNotFound = errors.New("network integration not found")

// Connection is safe for API responses. Credential bytes are stored separately and never serialized.
type Connection struct {
	ID                  string          `json:"id"`
	Label               string          `json:"label"`
	Provider            string          `json:"provider"`
	Config              json.RawMessage `json:"config"`
	HasCredential       bool            `json:"has_credential"`
	Status              string          `json:"status"`
	LastChecked         *time.Time      `json:"last_checked,omitempty"`
	LastError           string          `json:"last_error,omitempty"`
	SyncIntervalSeconds int             `json:"sync_interval_seconds"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	Credential          []byte          `json:"-"`
}

type Resource struct {
	ID           string          `json:"id"`
	ConnectionID string          `json:"connection_id"`
	Provider     string          `json:"provider"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	State        string          `json:"state,omitempty"`
	Address      string          `json:"address,omitempty"`
	Details      json.RawMessage `json:"details"`
	ObservedAt   time.Time       `json:"observed_at"`
}

// Config uses provider-specific, non-secret settings only.
type Config struct {
	AccountID    string `json:"account_id,omitempty"`
	Tailnet      string `json:"tailnet,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	InventoryURL string `json:"inventory_url,omitempty"`
}

func (c Connection) Validate() error {
	if strings.TrimSpace(c.Label) == "" || len(c.Label) > 120 {
		return errors.New("label must be between 1 and 120 characters")
	}
	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(c.Config)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return errors.New("config must be a JSON object")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("config must contain exactly one JSON object")
	}
	if c.SyncIntervalSeconds < 60 || c.SyncIntervalSeconds > 86400 {
		return errors.New("inventory interval must be between 60 and 86400 seconds")
	}
	switch c.Provider {
	case CloudflareTunnel:
		if !identifier(cfg.AccountID, 32) {
			return errors.New("Cloudflare account id is required")
		}
	case Tailscale:
		if strings.TrimSpace(cfg.Tailnet) != cfg.Tailnet || cfg.Tailnet == "" || len(cfg.Tailnet) > 253 || strings.ContainsAny(cfg.Tailnet, "/?#\\ \t\r\n") {
			return errors.New("Tailscale tailnet is required")
		}
		if strings.TrimSpace(cfg.ClientID) != cfg.ClientID || cfg.ClientID == "" || len(cfg.ClientID) > 200 {
			return errors.New("Tailscale OAuth client id is required")
		}
	case Custom:
		u, err := url.Parse(cfg.InventoryURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
			return errors.New("custom inventory URL must use HTTPS (HTTP is allowed only on loopback)")
		}
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("custom inventory URL cannot contain credentials, query parameters, or a fragment")
		}
	default:
		return fmt.Errorf("unsupported network provider %q", c.Provider)
	}
	return nil
}

func identifier(s string, n int) bool {
	if len(s) < 1 || len(s) > n {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
