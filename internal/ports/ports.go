// Package ports are the host ports: what only the running panel can give an entity, because it
// needs the panel's own keys or its connection to the hosts. An entity uses one with
// mcpl.Need[ports.X](c); the panel provides each once (internal/api/mcpl_services.go), so a new
// entity that uses them needs no change outside its folder.
//
// What an entity can build from its own package (a Postgres repository, an HTTP client) is not a
// port: the entity provides it itself with `var Services = mcpl.Provider(...)`.
//
// A port is added here only for a capability several entities could share; one entity's need is a
// small interface in that entity's package, provided by its own Services.
package ports

import "context"

// Sealer is the panel's at-rest encryption. Callers pass distinct associated data for each purpose,
// so a box sealed for one thing cannot be opened as another.
type Sealer interface {
	Seal(plain []byte, aad string) ([]byte, error)
	Open(box []byte, aad string) ([]byte, error)
}

// GitHub is the panel's GitHub App, which organizations install on their GitHub accounts. Which
// installation belongs to which organization is in github_installations; a port call trusts its
// caller to have checked that.
type GitHub interface {
	// Repositories are the repositories one installation can reach.
	Repositories(ctx context.Context, installation int64) ([]GitHubRepository, error)
}

// GitHubRepository is one repository an installation can reach.
type GitHubRepository struct {
	ID       int64
	FullName string
	Private  bool
}

// Workspaces is the runtime's view of the workspaces people create.
type Workspaces interface {
	// GiveKeys sets the SSH keys on every workspace the person created and applies them to the running
	// ones now; the rest get them when they next start.
	GiveKeys(ctx context.Context, sub string, keys []string) error
}
