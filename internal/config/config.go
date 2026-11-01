// Package config loads console settings from a JSON file plus env overrides.
package config

import (
	"encoding/json"
	"errors"
	"os"

	"shippin.cloud/console/internal/approval"
)

type Brand struct {
	Name    string `json:"name"`
	Short   string `json:"short"`
	Tagline string `json:"tagline"`
}

type IDP struct {
	Name     string `json:"name"` // product name users see, e.g. "Nomen" (the IdP behind Issuer)
	Issuer   string `json:"issuer"`
	ClientID string `json:"client_id"`
	Dial     string `json:"dial"`
	CAFile   string `json:"ca_file"`
	// AccountPath is the IdP self-service page (MFA, profile) under Issuer.
	AccountPath string `json:"account_path"`
	// Providers are upstream sign-in buttons (Google, GitHub...) configured as IdPs inside the
	// IdP. /auth/login?idp=<ID> asks the IdP to jump straight to that provider.
	Providers []IdPProvider `json:"providers"`
}

// IdPProvider is one upstream login offered on the sign-in page.
type IdPProvider struct {
	ID    string `json:"id"`     // short key used in /auth/login?idp=, e.g. "google"
	Label string `json:"label"`  // button text, e.g. "Google"
	IdPID string `json:"idp_id"` // the IdP's internal id for this upstream provider
}

// Provider returns the configured upstream provider with the given id.
func (i IDP) Provider(id string) (IdPProvider, bool) {
	for _, p := range i.Providers {
		if p.ID == id && p.IdPID != "" {
			return p, true
		}
	}
	return IdPProvider{}, false
}

type Incus struct {
	HostdURL  string `json:"hostd_url"`
	Project   string `json:"project"`
	HostLabel string `json:"host_label"`
}

// Workspaces holds workspace defaults (connect user, SSH port range start, plan).
type Workspaces struct {
	User        string `json:"user"`
	PortMin     int    `json:"port_min"`
	PortMax     int    `json:"port_max"`
	DefaultPlan string `json:"default_plan"`
	CodeDir     string `json:"code_dir"`
	BaseDomain  string `json:"base_domain"`
	IDEPort     int    `json:"ide_port"`
	// Sizes offered in the create flow; clamped by the plan's project limits in Incus.
	Sizes []Size `json:"sizes"`
	// TemplateSeed is a JSON file of templates inserted (by slug) at startup if missing.
	TemplateSeed string `json:"template_seed"`
	PreviewMin   int    `json:"preview_min"`
	PreviewMax   int    `json:"preview_max"`
	ImagePrefix  string `json:"image_prefix"`
	Guards
}

// Onboarding lets a signed-in user with no tenant create their own org + tenant.
type Onboarding struct {
	Enabled bool   `json:"enabled"`
	Plan    string `json:"plan"`
}

type Probe struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	URL      string `json:"url,omitempty"`
	Host     string `json:"host,omitempty"`
	TCP      string `json:"tcp,omitempty"`
	Instance string `json:"instance"`
}

type Config struct {
	Brand             Brand             `json:"brand"`
	Domain            string            `json:"domain"`
	GovernanceDomains []string          `json:"governance_domains"`
	ConsoleHost       string            `json:"console_host"`
	IDP               IDP               `json:"idp"`
	Incus             Incus             `json:"incus"`
	Probes            []Probe           `json:"probes"`
	PlanName          string            `json:"plan_name"`
	Workspaces        Workspaces        `json:"workspaces"`
	Onboarding        Onboarding        `json:"onboarding"`
	IDPServiceToken   string            `json:"-"`
	IDPReadToken      string            `json:"-"`
	Operator          Operator          `json:"operator"`
	Roles             Roles             `json:"roles"`
	Access            Access            `json:"access"`
	Staff             Staff             `json:"staff"`
	Identity          Identity          `json:"identity"`
	Mail              Mail              `json:"mail"`
	Jobs              Jobs              `json:"jobs"`
	AI                AI                `json:"ai"`
	GitHub            GitHub            `json:"github"`
	Connect           Connect           `json:"connect"`
	Site              Site              `json:"site"`
	TenantDB          TenantDB          `json:"tenant_db"`
	TenantDBSecrets   map[string]string `json:"-"`
	Migrations        Migrations        `json:"migrations"` // hold: files a change ticket releases later
	Approval          approval.Policy   `json:"approval"`
	DatabaseURL       string            `json:"-"`
	SessionKey        string            `json:"-"`
	Listen            string            `json:"-"`
	StaticDir         string            `json:"-"`
	BuildSections
}

// Load reads SHIPPIN_CONFIG (default config/shippin.example.json) and env overrides.
func Load() (*Config, error) {
	path := env("SHIPPIN_CONFIG", "config/shippin.example.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(raw, c); err != nil {
		return nil, err
	}
	if v := os.Getenv("SHIPPIN_OIDC_CLIENT_ID"); v != "" {
		c.IDP.ClientID = v
	}
	if v := os.Getenv("SHIPPIN_OPS_OIDC_CLIENT_ID"); v != "" {
		c.Operator.ClientID = v
	}
	if c.Domain == "" {
		c.Domain = "shippin.cloud"
	}
	if len(c.GovernanceDomains) == 0 {
		c.GovernanceDomains = []string{c.Domain}
	}
	c.DatabaseURL = os.Getenv("SHIPPIN_DATABASE_URL")
	c.SessionKey = os.Getenv("SHIPPIN_SESSION_KEY")
	c.IDPServiceToken = os.Getenv("SHIPPIN_IDP_SERVICE_TOKEN")
	c.IDPReadToken = os.Getenv("SHIPPIN_IDP_READ_TOKEN")
	c.Staff.TeamToken = os.Getenv("SHIPPIN_IDP_TEAM_TOKEN")
	c.Listen = env("SHIPPIN_LISTEN", ":8080")
	c.StaticDir = env("SHIPPIN_STATIC", "web/dist")
	c.Mail.load()
	c.GitHub.load()
	c.Authz.load()
	c.Vaultix.load()
	c.Services.RRFlow.loadEnv()
	if c.Roles.Infra == "" {
		c.Roles.Infra = "op_infra"
	}
	if len(c.Approval.Rules) == 0 {
		c.Approval = approval.Default()
	}
	c.Approval = c.Approval.WithRuntime(os.Getenv)
	if err := c.loadTenantDB(); err != nil {
		return nil, err
	}
	return c, c.Check()
}

// Check validates every fail-closed section.
func (c *Config) Check() error {
	for _, check := range []func() error{func() error { return c.Roles.Check(c.Operator.Role) }, c.Staff.Check, c.Access.Check, c.Mail.Check, c.Jobs.Check, c.AI.Check, c.GitHub.Check, c.Prompt.Check, c.Agents.Check, c.Authz.Check, c.Vaultix.Check, c.Bridge.Check, c.Chat.Check, c.Connect.Check, c.Servers.Check, c.Orgs.Check} {
		if err := check(); err != nil {
			return err
		}
	}
	c.IdentityDefaults()
	if c.IDP.Name == "" {
		return errors.New("idp.name is required (the identity product name users see)")
	}
	if err := c.Sparks.Check(c.Operator.AdminHosts); err != nil {
		return err
	}
	return c.Workspaces.Check()
}
