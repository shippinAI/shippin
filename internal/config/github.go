package config

import (
	"encoding/base64"
	"errors"
	"os"
	"regexp"
	"strings"
)

// GitHub configures M3 import through the Shippin GitHub App. The App's credentials come only from
// the environment (the site's secret store → SHIPPIN_GITHUB_*), never from this file.
type GitHub struct {
	Enabled bool   `json:"enabled"`
	AppSlug string `json:"app_slug"`
	APIBase string `json:"api_base"`
	WebBase string `json:"web_base"`
	// Tenants limits the feature during rollout; empty means every tenant.
	Tenants []string `json:"tenants"`
	// RefreshMinutes re-pushes installation tokens into running imported workspaces (tokens last 1 h).
	RefreshMinutes int `json:"refresh_minutes"`
	// PollMinutes reconciles installations from the API when webhooks can't reach the console.
	PollMinutes  int `json:"poll_minutes"`
	StateMinutes int `json:"state_minutes"`
	// MaxRepos caps the picker listing (pages of 100).
	MaxRepos int `json:"max_repos"`
	// TokenPermissions narrow each workspace's 1-hour installation token (one repo, these scopes).
	TokenPermissions map[string]string `json:"token_permissions"`

	AppID         string `json:"-"`
	ClientID      string `json:"-"`
	ClientSecret  string `json:"-"`
	PrivateKey    string `json:"-"`
	WebhookSecret string `json:"-"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,33}$`)

func (g *GitHub) load() {
	g.AppID = os.Getenv("SHIPPIN_GITHUB_APP_ID")
	g.ClientID = os.Getenv("SHIPPIN_GITHUB_CLIENT_ID")
	g.ClientSecret = os.Getenv("SHIPPIN_GITHUB_CLIENT_SECRET")
	g.PrivateKey = pemOrBase64(os.Getenv("SHIPPIN_GITHUB_PRIVATE_KEY"))
	g.WebhookSecret = os.Getenv("SHIPPIN_GITHUB_WEBHOOK_SECRET")
}

// pemOrBase64 accepts the App key as PEM or as one-line base64 of the PEM (systemd env files
// can't hold the PEM's newlines). A value that decodes to nothing PEM-like is kept as given.
func pemOrBase64(v string) string {
	if v == "" || strings.HasPrefix(v, "-----BEGIN") {
		return v
	}
	if b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v)); err == nil && strings.HasPrefix(string(b), "-----BEGIN") {
		return string(b)
	}
	return v
}

// Allows reports whether tenant may use GitHub import during rollout.
func (g GitHub) Allows(tenant string) bool {
	if !g.Enabled {
		return false
	}
	for _, t := range g.Tenants {
		if t == tenant {
			return true
		}
	}
	return len(g.Tenants) == 0
}

// Check fails closed when enabled without the App's settings or credentials.
func (g GitHub) Check() error {
	if !g.Enabled {
		return nil
	}
	switch {
	case !slugRe.MatchString(g.AppSlug):
		return errors.New("github.app_slug must be the App's URL slug")
	case !strings.HasPrefix(g.APIBase, "https://") || !strings.HasPrefix(g.WebBase, "https://"):
		return errors.New("github.api_base and github.web_base must be https URLs")
	case g.RefreshMinutes < 1 || g.RefreshMinutes > 50 || g.PollMinutes < 1 || g.StateMinutes < 1 || g.MaxRepos < 1:
		return errors.New("github.refresh_minutes (1-50), poll_minutes, state_minutes and max_repos must be set")
	case len(g.TokenPermissions) == 0:
		return errors.New("github.token_permissions must list the workspace token scopes")
	case g.ClientID == "" || g.ClientSecret == "" || g.PrivateKey == "" || g.WebhookSecret == "":
		return errors.New("github is enabled but SHIPPIN_GITHUB_CLIENT_ID, _CLIENT_SECRET, _PRIVATE_KEY or _WEBHOOK_SECRET is missing")
	}
	return nil
}
