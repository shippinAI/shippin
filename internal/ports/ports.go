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
	// JITConfig registers a single-use runner on one repository with these labels and returns its
	// encoded configuration and GitHub's id for the runner. The configuration is a credential: it goes
	// to the runner's first boot only and is never logged, stored or returned by an action.
	JITConfig(ctx context.Context, installation, repository int64, fullName, runnerName string, labels []string) (config string, runnerID int64, err error)
}

// GitHubRepository is one repository an installation can reach.
type GitHubRepository struct {
	ID       int64
	FullName string
	Private  bool
}

// Machines are single-use virtual machines on the provider's hosts: a build runner for one job now,
// anything else that must start clean and be removed after later.
type Machines interface {
	// Hosts are the servers a machine may be placed on, in the order to prefer when equally loaded.
	Hosts(ctx context.Context) ([]string, error)
	// Create makes and starts a virtual machine on one of those servers and returns the hypervisor's id
	// for it. A machine that was made but did not start is still there to Delete.
	Create(ctx context.Context, server string, m Machine) (guest string, err error)
	// Delete removes a machine. One the hypervisor confirms is already gone is not an error.
	Delete(ctx context.Context, server, guest string) error
}

// Machine is a virtual machine to make.
type Machine struct {
	Name      string
	Image     string
	Cores     int
	MemoryMiB int
	DiskGiB   int
	// UserData is its cloud-init user data, which may carry a credential for its first boot.
	UserData string
}

// Workspaces is the runtime's view of the workspaces people create.
type Workspaces interface {
	// GiveKeys sets the SSH keys on every workspace the person created and applies them to the running
	// ones now; the rest get them when they next start.
	GiveKeys(ctx context.Context, sub string, keys []string) error
}
