// Package hv is the console's hypervisor driver surface.
//
// Proxmox and Incus are the two drivers. Bare is a host with no hypervisor API.
// A mutating call returns the guest the API reported after the call. It does not
// invent a status. See docs/design/hypervisor-drivers.md.
package hv

import (
	"context"
	"errors"
	"time"
)

// Kind of a guest.
const (
	KindVM = "vm"
	KindCT = "ct"
)

// ErrNoHypervisor is returned by the bare driver for every call.
var ErrNoHypervisor = errors.New("hv: this server has no hypervisor API")

// ErrGuestNotFound means the provider confirmed that an addressed guest is absent.
// Callers must not infer absence from an arbitrary API or network error.
var ErrGuestNotFound = errors.New("hv: guest not found")

// GuestSpec is a guest to make. A driver uses what applies to it, and zero leaves its default.
type GuestSpec struct {
	Kind    string
	Name    string
	Image   string
	Storage string
	Network string
	Memory  int // MiB
	Cores   int
	Disk    int // GiB
	Start   bool
}

// Image is something a guest can be made from.
type Image struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"` // vm or ct; empty suits either
}

// GuestManager is a driver that can make and remove guests.
type GuestManager interface {
	Images(ctx context.Context) ([]Image, error)
	CreateGuest(ctx context.Context, spec GuestSpec) (Guest, error)
	DeleteGuest(ctx context.Context, id string) error
}

// SnapshotKeeper is a driver that can list and restore the snapshots it takes.
type SnapshotKeeper interface {
	Snapshots(ctx context.Context, id string) ([]string, error)
	Rollback(ctx context.Context, id, name string) (Guest, error)
}

// ErrUnsupported is a real gap, not a success. Console and exec are in this set.
var ErrUnsupported = errors.New("hv: not implemented for this driver")

// Guest is one VM or container as the hypervisor last reported it.
type Guest struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Kind     string        `json:"kind"`
	Status   string        `json:"status"`
	Node     string        `json:"node"`
	Tags     string        `json:"tags,omitempty"`
	Pool     string        `json:"pool,omitempty"`
	Template bool          `json:"template,omitempty"`
	Uptime   int64         `json:"uptime_seconds,omitempty"`
	Metrics  *GuestMetrics `json:"metrics,omitempty"`
	// Self marks the guest this panel runs on, so nothing stops it by accident.
	Self bool `json:"self,omitempty"`
}

// GuestNetworkObserver reads addresses reported by the hypervisor or its guest agent.
// These addresses describe provider observation only; they do not prove that a
// control-plane probe can route to the guest.
type GuestNetworkObserver interface {
	GuestNetwork(ctx context.Context, guest Guest) ([]GuestAddress, error)
}

// GuestAddress is one address returned by the provider for a guest interface.
type GuestAddress struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Family    string `json:"family"`
	Scope     string `json:"scope,omitempty"`
}

// GuestMetrics are provider-reported resource counters. Missing metrics stay absent; the API
// never fabricates zeros for a provider that cannot report them.
type GuestMetrics struct {
	CPU        float64 `json:"cpu_fraction"`
	CPUCount   int     `json:"cpu_count,omitempty"`
	Memory     int64   `json:"memory_used_bytes"`
	MaxMemory  int64   `json:"memory_total_bytes"`
	Disk       int64   `json:"disk_used_bytes"`
	MaxDisk    int64   `json:"disk_total_bytes"`
	NetworkIn  int64   `json:"network_in_bytes"`
	NetworkOut int64   `json:"network_out_bytes"`
}

// Volume is one storage pool or volume.
type Volume struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Node    string `json:"node"`
	Content string `json:"content,omitempty"`
	Used    int64  `json:"used_bytes"`
	Total   int64  `json:"total_bytes"`
}

// Network is one bridge or network.
type Network struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Node string `json:"node"`
}

// Health is a probe. Online is true only when the API answered.
type Health struct {
	Online  bool      `json:"online"`
	Version string    `json:"version,omitempty"`
	Error   string    `json:"error,omitempty"`
	Checked time.Time `json:"checked"`
}

// Driver reads and changes one hypervisor.
type Driver interface {
	Kind() string
	Health(ctx context.Context) (Health, error)
	Guests(ctx context.Context) ([]Guest, error)
	Guest(ctx context.Context, id string) (Guest, error)
	Start(ctx context.Context, id string) (Guest, error)
	Stop(ctx context.Context, id string) (Guest, error)
	Shutdown(ctx context.Context, id string) (Guest, error)
	Reboot(ctx context.Context, id string) (Guest, error)
	Snapshot(ctx context.Context, id, name string) (Guest, error)
	Storage(ctx context.Context) ([]Volume, error)
	Networks(ctx context.Context) ([]Network, error)
}
