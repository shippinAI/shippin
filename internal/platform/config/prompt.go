package config

import (
	"errors"
	"regexp"
	"strings"
)

// Prompt configures M4 prompt → project: a prompt box creates the project and runs the user's OWN
// signed-in CLI agent (Claude Code, Codex…) in their workspace. No LLM runs in the control plane.
type Prompt struct {
	Enabled bool `json:"enabled"`
	// Tenants limits the feature during rollout; empty means every tenant (AI must be allowed too).
	Tenants []string `json:"tenants"`
	// MaxBytes caps the prompt (hostd caps the file too).
	MaxBytes int `json:"max_bytes"`
	// Agents are the ai.providers ids that can build (each needs agent.providers in hostd.json).
	Agents []string `json:"agents"`
	// Stacks map words in a prompt to a template slug; the first stack with a hit wins. The user can
	// always pick one. DefaultTemplate is used when nothing matches.
	Stacks          []Stack `json:"stacks"`
	DefaultTemplate string  `json:"default_template"`
	// TermPath is where the agent's terminal is published on the workspace host (hostd agent.term_base).
	TermPath string `json:"term_path"`
	// AttachRRFlow binds rrflow/state to every prompt-built devspace when services.rrflow is enabled.
	AttachRRFlow bool `json:"attach_rrflow"`
}

// Stack is one choice in the stack chips: a template slug, its label and the words that pick it.
type Stack struct {
	Template string   `json:"template"`
	Label    string   `json:"label"`
	Keywords []string `json:"keywords"`
}

// Services configures managed service instances (07-multi-tenancy); rrflow/state first.
type Services struct {
	RRFlow ServiceToggle `json:"rrflow"`
}

// ServiceToggle switches one managed service on, optionally for a rollout list of tenants.
type ServiceToggle struct {
	Enabled bool     `json:"enabled"`
	Tenants []string `json:"tenants"`
	// AutoAttach provisions and binds the service when a project devspace is created.
	AutoAttach bool `json:"auto_attach"`
	// ListedOnly: an empty tenant list means no tenant (not every tenant), so a rollout can never
	// widen to everyone by accident (GC-20261019-PB B1: tenants come only from the list or the env).
	ListedOnly bool `json:"listed_only"`
}

var (
	templateSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	termPath     = regexp.MustCompile(`^/_[a-z]{2,16}/$`)
)

func allows(enabled bool, list []string, tenant string) bool {
	if !enabled {
		return false
	}
	for _, t := range list {
		if t == tenant {
			return true
		}
	}
	return len(list) == 0
}

// Allows reports whether tenant may create projects from a prompt.
func (p Prompt) Allows(tenant string) bool { return allows(p.Enabled, p.Tenants, tenant) }

// Allows reports whether tenant may attach this service.
func (s ServiceToggle) Allows(tenant string) bool {
	if s.ListedOnly && len(s.Tenants) == 0 {
		return false
	}
	return allows(s.Enabled, s.Tenants, tenant)
}

// AutoAttaches reports whether new project devspaces of tenant get the service bound automatically.
func (s ServiceToggle) AutoAttaches(tenant string) bool { return s.AutoAttach && s.Allows(tenant) }

// IsAgent reports whether provider id can run a build.
func (p Prompt) IsAgent(id string) bool {
	for _, a := range p.Agents {
		if a == id {
			return true
		}
	}
	return false
}

// Check fails closed on an enabled but malformed section.
func (p Prompt) Check() error {
	if !p.Enabled {
		return nil
	}
	if p.MaxBytes < 256 || p.MaxBytes > 64<<10 || len(p.Agents) == 0 || !templateSlug.MatchString(p.DefaultTemplate) {
		return errors.New("prompt.max_bytes (256-65536), prompt.agents and prompt.default_template are required")
	}
	if !termPath.MatchString(p.TermPath) {
		return errors.New("prompt.term_path must look like /_term/")
	}
	for _, s := range p.Stacks {
		if !templateSlug.MatchString(s.Template) || strings.TrimSpace(s.Label) == "" {
			return errors.New("prompt.stacks need a template slug and a label")
		}
	}
	return nil
}

// BuildSections (embedded in Config, so JSON stays flat) holds the M4 prompt, managed-service and N2 sections.
type BuildSections struct {
	Prompt   Prompt   `json:"prompt"`
	Services Services `json:"services"`
	Agents   Agents   `json:"agents"`
	Authz    Authz    `json:"authz"`
	Vaultix  Vaultix  `json:"vaultix"`
	Bridge   Bridge   `json:"bridge"`
	Bundle   Bundle   `json:"bundle"`
	Chat     Chat     `json:"chat"`
	Sparks   Sparks   `json:"sparks"`
	Servers  Servers  `json:"servers"`
	Orgs     Orgs     `json:"orgs"`
}
