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

// ErrUnsupported is a real gap, not a success. Console and exec are in this set.
var ErrUnsupported = errors.New("hv: not implemented for this driver")

// Guest is one VM or container as the hypervisor last reported it.
type Guest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Node   string `json:"node"`
	Tags   string `json:"tags,omitempty"`
	// Self marks the guest this panel runs on, so nothing stops it by accident.
	Self bool `json:"self,omitempty"`
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
