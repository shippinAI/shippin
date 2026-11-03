// Policy — what a Client may do.
//
// Loaded from /etc/warden/clients/<slug>/policy.yaml. Each client's policy
// declares what hosts they can act on, what tools they can invoke, and
// which tier-gated capabilities are unlocked.
//
// Phase 1 (this commit): minimal types + a permissive "system" policy so
// system-owned hosts always pass gates. Real enforcement lands in Phase 3.

package client

// Capability is a fine-grained permission grant.
//
// Examples:
//   - "host.read"           — list / view hosts
//   - "host.write"          — mutate host config
//   - "deploy.create"       — kick off new deployments
//   - "memory.search.global" — premium-only: cross-project memory search
//
// Adding a new capability = add a constant here + check it at the
// enforcement boundary (Phase 3).
type Capability string

const (
	CapHostRead   Capability = "host.read"
	CapHostWrite  Capability = "host.write"
	CapDeployRead Capability = "deploy.read"
	CapDeployRun  Capability = "deploy.run"

	CapMemoryRead         Capability = "memory.read"
	CapMemoryWrite        Capability = "memory.write"
	CapMemorySearchGlobal Capability = "memory.search.global" // premium
)

// Policy is the set of capabilities a Client holds plus host scoping.
type Policy struct {
	// Capabilities is the explicit grant set. Wildcard "*" grants all.
	Capabilities []Capability `yaml:"capabilities"`

	// HostsAllowed restricts which hosts this Client may touch. Empty
	// or nil means "only hosts owned by this client (HostsOwned)".
	// Wildcard "*" means "any host the system allows them to see".
	HostsAllowed []string `yaml:"hosts_allowed,omitempty"`
}

// Has reports whether p grants cap.
func (p *Policy) Has(cap Capability) bool {
	if p == nil {
		return false
	}
	for _, c := range p.Capabilities {
		if c == cap || c == "*" {
			return true
		}
	}
	return false
}

// SystemPolicy is the implicit policy for the system tenant. Permissive
// by design — the operator's own infrastructure is not gated by client
// policy.
var SystemPolicy = &Policy{
	Capabilities: []Capability{"*"},
	HostsAllowed: []string{"*"},
}
