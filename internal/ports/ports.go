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

import (
	"context"
	"errors"
	"time"

	"shippin.cloud/console/internal/clickhouse"
)

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

// Secrets is the installation's secret store: its Vaultix account, connected in the panel
// (docs/settings.md). Keys entered in the panel are kept there and nowhere else; the database keeps
// only a SecretRef.
type Secrets interface {
	// Where is the folder new keys are written to. It fails with ErrNoSecretStore while none is
	// connected.
	Where(ctx context.Context) (SecretRef, error)
	// Write creates the key ref names with value. A name is written once: a changed value gets a new
	// name, so the one in use is never overwritten.
	Write(ctx context.Context, ref SecretRef, value string) error
	// Read returns a key's value. Callers never log or return it.
	Read(ctx context.Context, ref SecretRef) (string, error)
	// Delete removes a key. One that is already gone is not an error.
	Delete(ctx context.Context, ref SecretRef) error
}

// SecretRef is where one key is kept, never its value.
type SecretRef struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Path        string `json:"path"`
	Name        string `json:"name"`
}

// ErrNoSecretStore is Secrets.Where with no secret store connected.
var ErrNoSecretStore = errors.New("no secret store is connected: connect Vaultix in Settings > Secret store first")

// Workspaces is the runtime's view of the workspaces people create.
type Workspaces interface {
	// GiveKeys sets the SSH keys on every workspace the person created and applies them to the running
	// ones now; the rest get them when they next start.
	GiveKeys(ctx context.Context, sub string, keys []string) error
}

// Guests is what the hosts run for an organization: its development spaces as the hypervisor has
// them now. Today the host agent answers; the provider drivers of internal/hv replace it behind this
// port (docs/reference/devspace-engine.md).
type Guests interface {
	// Inventory is the organization's guests by name. complete is false when a host could not be read
	// or answered in part: nothing is then known about a guest that is not listed.
	Inventory(ctx context.Context, org string) (guests map[string]Guest, complete bool)
	// Limits are what one guest on a plan may use, zero where the host has none for it.
	Limits(ctx context.Context, plan string) GuestLimits
}

// Guest is one machine as its host reports it.
type Guest struct {
	// Status is the hypervisor's word for it: Running, Stopped and others.
	Status      string
	IPv4        string
	MemoryBytes int64
}

// GuestLimits are a plan's limits for one guest.
type GuestLimits struct {
	CPU    int    `json:"workspace_cpu"`
	Memory string `json:"workspace_memory"`
	Home   int    `json:"home_gib"`
	Root   int    `json:"root_gib"`
}

// Metrics is the panel's metrics history (OBS-01, docs/metrics.md): the ClickHouse it is connected to
// now, which an owner can change or turn off while the panel runs, and what this panel process's
// sampler is doing.
type Metrics interface {
	// ClickHouse is the connected store, or nil while there is none.
	ClickHouse() *clickhouse.Client
	// Sampling is this process's sampler as it last ran.
	Sampling() Sampling
}

// Sampling is what one panel process's metrics sampler is doing and why.
type Sampling struct {
	// State is sampling (this process writes samples), standby (another process does), idle (it can't:
	// Reason says why), off (turned off in the settings) or starting.
	State           string
	Reason          string
	IntervalSeconds int
	// LastAt is when it last wrote samples; zero before the first.
	LastAt time.Time
	// LastRows is how many samples it last wrote, and Failed the hosts that could not be read then.
	LastRows int
	Failed   []string
}
