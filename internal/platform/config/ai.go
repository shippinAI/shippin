package config

import (
	"errors"
	"net/url"
	"regexp"
)

// AI configures M2 subscription activation: each user signs in to their OWN AI subscription with
// the vendor's CLI inside their own activation container. Shippin never sees the credentials.
type AI struct {
	Enabled bool `json:"enabled"`
	// Providers are the vendor CLIs preinstalled in the workspace image, in display order.
	Providers []AIProvider `json:"providers"`
	// IdleMinutes stops an activation container this long after the user last had it open.
	IdleMinutes int `json:"idle_minutes"`
	// TouchSeconds is how often an open activation terminal reports that it is still in use.
	TouchSeconds int `json:"touch_seconds"`
	// Tenants limits activation to these tenant ids while rolling out; empty means every tenant.
	Tenants []string `json:"tenants"`
	// EgressHosts documents the vendor hosts the CLIs need (enforced where egress allow-lists exist).
	EgressHosts []string `json:"egress_hosts"`
	// TermsNote is the one-line note shown before activation.
	TermsNote string `json:"terms_note"`
}

// AIProvider is one vendor CLI. LoginCommand is what the user runs in their terminal.
type AIProvider struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	LoginCommand string `json:"login_command"`
	Vendor       string `json:"vendor"`
	DeviceURL    string `json:"device_url,omitempty"`
}

var providerID = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)

// Provider returns the configured provider with id.
func (a AI) Provider(id string) (AIProvider, bool) {
	for _, p := range a.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return AIProvider{}, false
}

// Allows reports whether tenant may use activation during rollout.
func (a AI) Allows(tenant string) bool {
	if !a.Enabled {
		return false
	}
	if len(a.Tenants) == 0 {
		return true
	}
	for _, t := range a.Tenants {
		if t == tenant {
			return true
		}
	}
	return false
}

// Check fails closed on a malformed enabled section.
func (a AI) Check() error {
	if !a.Enabled {
		return nil
	}
	if len(a.Providers) == 0 || a.IdleMinutes < 1 || a.TouchSeconds < 1 {
		return errors.New("ai.providers, ai.idle_minutes and ai.touch_seconds must be set")
	}
	seen := map[string]bool{}
	for _, p := range a.Providers {
		if !providerID.MatchString(p.ID) || p.Label == "" || p.LoginCommand == "" || seen[p.ID] {
			return errors.New("ai.providers need a unique id, a label and a login_command")
		}
		seen[p.ID] = true
		if p.DeviceURL != "" {
			u, err := url.Parse(p.DeviceURL)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return errors.New("ai device_url must be a public HTTPS sign-in page without credentials or query parameters")
			}
		}
	}
	return nil
}
