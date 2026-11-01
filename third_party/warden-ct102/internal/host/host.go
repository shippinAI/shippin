// Package host defines the multi-host abstraction for Warden.
//
// A Host is a managed instance — Proxmox node, Wardenclyffe-fork node,
// or bare SSH target. Hosts are loaded from a YAML registry and dispatched
// to the right Connector implementation under kinds/.
//
// See docs/decisions/0008-repo-foundation.md §2 for the full spec.
package host

import (
	"context"
	"time"
)

// Kind identifies which Connector implementation drives a Host.
//
// Add a new kind = drop a new directory under kinds/ + register in
// registry.go's dispatch map. Do not branch on Kind in feature code;
// always go through the Connector interface.
type Kind string

const (
	KindProxmox      Kind = "proxmox"      // stock Proxmox VE 8.x / 9.x
	KindWardenclyffe Kind = "wardenclyffe" // our own fork (extends proxmox initially)
	KindBare         Kind = "bare"         // SSH-only, no panel
)

// IsKnown reports whether k is a registered host kind.
func (k Kind) IsKnown() bool {
	switch k {
	case KindProxmox, KindWardenclyffe, KindBare:
		return true
	}
	return false
}

// NetAddr is one network address attached to a Host. A single Host may
// have several (public API, internal IP, SSH endpoint, console URL).
type NetAddr struct {
	Role    string `yaml:"role"`    // "api" | "ssh" | "console" | "internal"
	Host    string `yaml:"host"`    // hostname or IP
	Port    int    `yaml:"port"`    // 0 = default for the role
	Verify  bool   `yaml:"verify"`  // verify TLS on this address
	Network string `yaml:"network"` // "public" | "internal" — informational
}

// CredRefs is a set of credential REFERENCES (never bytes).
//
// Each value is a path inside the keystore tree (ADR 0009). The `host`
// package never reads secret bytes; that's the keystore's job.
type CredRefs struct {
	APIToken  string `yaml:"api_token,omitempty"`  // e.g. "platforms/proxmox/api.token"
	SSHKey    string `yaml:"ssh_key,omitempty"`    // e.g. "ssh/warden-to-lxc.key"
	BasicAuth string `yaml:"basic_auth,omitempty"` // for Caddy/Traefik dashboard auth
}

// Host is a single managed instance. Loaded from /etc/warden/hosts/<name>/host.yaml.
type Host struct {
	Name        string    `yaml:"name"`               // slug; matches the directory name
	Kind        Kind      `yaml:"kind"`               // drives Connector dispatch
	Provider    string    `yaml:"provider,omitempty"` // explicit provider root id (e.g. proxmox-x)
	OS          string    `yaml:"os"`                 // e.g. "Debian 12", "Ubuntu 24.04"
	Addresses   []NetAddr `yaml:"addresses"`          // at least one; first matching role wins
	OwnerClient string    `yaml:"owner_client"`       // client slug; "" = system-owned
	Tenant      string    `yaml:"tenant,omitempty"`   // clyffe tenant slug (if customer-owned)
	VLAN        string    `yaml:"vlan,omitempty"`     // VLAN segment label/id
	Credentials CredRefs  `yaml:"credentials"`        // refs into the keystore
	Tags        []string  `yaml:"tags"`               // free-form classification

	// Cached state — populated from connector probes, not from YAML.
	LastProbe Status `yaml:"-"`
}

// Status is the cached result of a connector Probe call.
type Status struct {
	Online      bool      `json:"online"`
	Version     string    `json:"version"`
	Uptime      string    `json:"uptime,omitempty"`
	LastChecked time.Time `json:"last_checked"`
	LastError   string    `json:"last_error,omitempty"`
}

// Inventory is the cached snapshot of LXC/VM/storage on a Host.
// Connector implementations populate this; consumers read it.
type Inventory struct {
	LXCs    []ResourceRef `json:"lxcs"`
	VMs     []ResourceRef `json:"vms"`
	Storage []StorageRef  `json:"storage"`
	Fetched time.Time     `json:"fetched_at"`
}

// ResourceRef is the minimal shape every Connector returns for an LXC or VM.
type ResourceRef struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"` // "running" | "stopped" | "paused" | …
	Node   string `json:"node"`
	Tags   string `json:"tags,omitempty"`
}

// StorageRef is the minimal shape every Connector returns for storage.
type StorageRef struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Used    int64  `json:"used_bytes"`
	Total   int64  `json:"total_bytes"`
	Node    string `json:"node"`
	Content string `json:"content,omitempty"` // "images,iso,vztmpl" etc.
}

// Connector is the interface every host Kind implements.
//
// Concrete implementations live in kinds/<kind>/connector.go. They are
// constructed via the registry's dispatch — feature code never calls
// kind constructors directly.
type Connector interface {
	// Probe verifies connectivity + auth. Cheap. Called frequently.
	Probe(ctx context.Context) (Status, error)

	// Inventory snapshots LXCs/VMs/storage. Expensive. Cache results.
	Inventory(ctx context.Context) (Inventory, error)
}
