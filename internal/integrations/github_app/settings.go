// Package github_app is the settings of the installation's GitHub App, which organizations install to
// import projects and run build runners. Its client secret, private key and webhook secret are keys:
// they go to the secret store.
package github_app

import (
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/gh"
	"shippin.cloud/console/internal/settings"
)

// App is the section as its users read it: the file's GitHub settings with the App's identifiers,
// which the file kept in the environment. Keys are read separately, with settings.Store.Key.
type App struct {
	config.GitHub
	AppID    string `json:"app_id"`
	ClientID string `json:"client_id"`
}

// Settings is the GitHub App.
var Settings = settings.Section{
	Key:     "github",
	Title:   "GitHub App",
	Summary: "The GitHub App organizations install to import projects and run build runners. Create it on GitHub, then enter its details here.",
	Fields: []settings.Field{
		{Name: "enabled", Label: "Use the GitHub App", Kind: settings.Bool},
		{Name: "app_slug", Label: "App name in its URL", Kind: settings.Text, Optional: true,
			Help: "The part after github.com/apps/ on the App's public page."},
		{Name: "app_id", Label: "App ID", Kind: settings.Text, Optional: true, Help: "On the App's settings page, under About."},
		{Name: "client_id", Label: "Client ID", Kind: settings.Text, Optional: true},
		{Name: "client_secret", Label: "Client secret", Kind: settings.Key, Env: "SHIPPIN_GITHUB_CLIENT_SECRET",
			Help: "Generated on the App's settings page. Saved to the secret store and never shown again."},
		{Name: "private_key", Label: "Private key", Kind: settings.LongKey, Env: "SHIPPIN_GITHUB_PRIVATE_KEY",
			Help: "The .pem file GitHub gives you when you generate a private key. Paste its whole contents.",
			Valid: func(pem string) error {
				_, err := gh.New("check", "check", pem, "https://api.github.com", "https://github.com")
				return err
			}},
		{Name: "webhook_secret", Label: "Webhook secret", Kind: settings.Key, Env: "SHIPPIN_GITHUB_WEBHOOK_SECRET",
			Help: "The secret you entered for the App's webhook. GitHub signs each delivery with it."},
		{Name: "api_base", Label: "GitHub API address", Kind: settings.URL, Help: "https://api.github.com, or your GitHub Enterprise API."},
		{Name: "web_base", Label: "GitHub web address", Kind: settings.URL, Help: "https://github.com, or your GitHub Enterprise address."},
		{Name: "tenants", Label: "Only these organizations", Kind: settings.List, Optional: true,
			Help: "Organization IDs that may use GitHub while you try it out. Empty lets every organization."},
		{Name: "refresh_minutes", Label: "Refresh workspace tokens every (minutes)", Kind: settings.Integer, Min: 1, Max: 50,
			Help: "GitHub tokens last an hour, so running workspaces get a new one before then."},
		{Name: "poll_minutes", Label: "Check installations every (minutes)", Kind: settings.Integer, Min: 1, Max: 1440,
			Help: "Catches installs and removals when GitHub's webhooks can't reach the panel."},
		{Name: "state_minutes", Label: "Install link lasts (minutes)", Kind: settings.Integer, Min: 1, Max: 120},
		{Name: "max_repos", Label: "Most repositories listed", Kind: settings.Integer, Min: 1, Max: 5000},
		{Name: "token_permissions", Label: "Workspace token permissions", Kind: settings.Map,
			Help: "What each workspace's one-repository token may do, such as contents: write."},
	},
	Seed: func(c *config.Config) any { return App{GitHub: c.GitHub, AppID: c.GitHub.AppID, ClientID: c.GitHub.ClientID} },
	SeedKeys: func(c *config.Config) map[string]string {
		return map[string]string{"client_secret": c.GitHub.ClientSecret, "private_key": c.GitHub.PrivateKey, "webhook_secret": c.GitHub.WebhookSecret}
	},
	Check: func(v settings.Values, c settings.Checking) error {
		a, err := settings.As[App](v)
		if err != nil {
			return err
		}
		g := a.GitHub
		g.AppID, g.ClientID = a.AppID, a.ClientID
		// Only whether each key is there matters to the check; its value was checked when entered.
		for name, set := range map[string]*string{"client_secret": &g.ClientSecret, "private_key": &g.PrivateKey, "webhook_secret": &g.WebhookSecret} {
			if c.Has(name) {
				*set = "set"
			}
		}
		return g.Check()
	},
}
