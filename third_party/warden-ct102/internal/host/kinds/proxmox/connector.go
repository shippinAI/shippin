// Package proxmox implements host.Connector for stock Proxmox VE 8.x / 9.x.
//
// Phase 2 of ADR 0008 imports the existing logic from warden/proxmox.go
// into this package. Until then this is a registered stub so the host
// registry compiles and reports a clear "not yet wired" error to callers.
package proxmox

import (
	"context"
	"errors"

	"warden/internal/host"
)

func init() {
	host.Register(host.KindProxmox, NewConnector)
}

// NewConnector constructs a Proxmox VE Connector for the given Host.
//
// Phase 2 will wire Host.Addresses + Host.Credentials into the Proxmox
// API client (proxmoxer-equivalent in Go). For now it returns a Connector
// whose methods report ErrNotImplemented.
func NewConnector(h *host.Host) (host.Connector, error) {
	if h == nil {
		return nil, errors.New("proxmox: nil host")
	}
	return &Connector{HostName: h.Name}, nil
}

// Connector is the Proxmox VE host kind. Stub fields; expanded in Phase 2.
type Connector struct {
	HostName string
}

// Compile-time interface conformance check.
var _ host.Connector = (*Connector)(nil)

// ErrNotImplemented is returned by every method until Phase 2 wires the
// real PVE API client.
var ErrNotImplemented = errors.New("proxmox connector: not yet implemented (ADR 0008 Phase 2)")

func (c *Connector) Probe(_ context.Context) (host.Status, error) {
	return host.Status{
		Online:    false,
		LastError: ErrNotImplemented.Error(),
	}, nil
}

func (c *Connector) Inventory(_ context.Context) (host.Inventory, error) {
	return host.Inventory{}, ErrNotImplemented
}
