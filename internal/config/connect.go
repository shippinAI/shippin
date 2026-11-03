package config

import (
	"errors"
	"strings"
)

// Connect is "Connect accounts": per-workspace sign-in for the CLIs the image ships. Pasted
// tokens are kept in Vaultix under the tenant's project at VaultixPath and pushed into the
// workspace on every start.
type Connect struct {
	Enabled bool `json:"enabled"`
	// VaultixEnv is the environment slug and VaultixPath the folder template ({workspace}).
	VaultixEnv  string            `json:"vaultix_env"`
	VaultixPath string            `json:"vaultix_path"`
	Providers   []ConnectProvider `json:"providers"`
	// IDEFolder is the folder the "Open terminal" IDE deep link opens.
	IDEFolder string `json:"ide_folder"`
}

// ConnectProvider is one card. VaultixKey names the secret; TokenHint explains what to paste.
type ConnectProvider struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Vendor     string `json:"vendor"`
	TokenLabel string `json:"token_label"`
	TokenHint  string `json:"token_hint"`
	TokenURL   string `json:"token_url"`
	VaultixKey string `json:"vaultix_key"`
	// BrowserNote says how the browser flow works (device code, or paste a code back).
	BrowserNote string `json:"browser_note"`
}

// Path is the provider secrets folder of one workspace.
func (c Connect) Path(workspace string) string {
	return strings.ReplaceAll(c.VaultixPath, "{workspace}", workspace)
}

// Provider returns the configured provider with id.
func (c Connect) Provider(id string) (ConnectProvider, bool) {
	for _, p := range c.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return ConnectProvider{}, false
}

func (c Connect) Check() error {
	if !c.Enabled {
		return nil
	}
	if c.VaultixEnv == "" || !strings.HasPrefix(c.VaultixPath, "/") || !strings.Contains(c.VaultixPath, "{workspace}") || c.IDEFolder == "" {
		return errors.New("connect: vaultix_env, vaultix_path (with {workspace}) and ide_folder are required")
	}
	seen := map[string]bool{}
	for _, p := range c.Providers {
		if !providerID.MatchString(p.ID) || p.Label == "" || p.VaultixKey == "" || seen[p.ID] {
			return errors.New("connect: each provider needs a unique id, a label and a vaultix_key")
		}
		seen[p.ID] = true
	}
	return nil
}
