// Package client defines the multi-tenant abstraction for Warden.
//
// A Client is a tenant — the entity that owns one or more Hosts. The
// system itself counts as the implicit "system" client; user-facing
// tenants get explicit Client records.
//
// See docs/decisions/0008-repo-foundation.md §4.
package client

import (
	"time"
)

// Tier classifies what this client may do. Matches the council tier
// model in ADR 0009 (memory governance).
type Tier string

const (
	TierDefault  Tier = "default"
	TierEnhanced Tier = "enhanced"
	TierPremium  Tier = "premium"
)

// IsKnown reports whether t is a registered tier.
func (t Tier) IsKnown() bool {
	switch t {
	case TierDefault, TierEnhanced, TierPremium:
		return true
	}
	return false
}

// Client is a tenant. Loaded from /etc/warden/clients/<slug>/client.yaml.
type Client struct {
	Slug       string    `yaml:"slug"`        // URL-safe identifier
	Name       string    `yaml:"name"`        // display name
	OwnerEmail string    `yaml:"owner_email"` // primary contact
	Tier       Tier      `yaml:"tier"`        // default | enhanced | premium
	HostsOwned []string  `yaml:"hosts_owned"` // host slugs this client owns
	PolicyRef  string    `yaml:"policy_ref"`  // path to policy.yaml (relative to client dir)
	CreatedAt  time.Time `yaml:"created_at"`
	UpdatedAt  time.Time `yaml:"updated_at"`
}

// IsSystem reports whether c is the implicit system tenant.
//
// The empty Client (or Slug == "") is treated as the system tenant; it
// owns infrastructure that the operator runs for themselves and is
// exempt from per-client policy gates.
func (c Client) IsSystem() bool {
	return c.Slug == "" || c.Slug == "system"
}
