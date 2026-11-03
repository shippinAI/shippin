// Package bare implements host.Connector for SSH-only, non-Proxmox hosts.
//
// Use cases:
//   - A standalone Linux box managed via SSH (no virtualization panel)
//   - A development VM you want Warden to surface but not orchestrate
//   - A future endpoint where Warden runs commands but doesn't touch a panel API
//
// Phase 1 (this commit): stub. Registered, returns "not yet implemented".
//
// Phase 2-3: SSH probe + remote `lxc list` / `virsh list` / equivalent
// to populate Inventory.

package bare

import (
	"context"
	"errors"

	"warden/internal/host"
)

func init() {
	host.Register(host.KindBare, NewConnector)
}

// NewConnector constructs a bare-host Connector.
func NewConnector(h *host.Host) (host.Connector, error) {
	if h == nil {
		return nil, errors.New("bare: nil host")
	}
	return &Connector{HostName: h.Name}, nil
}

// Connector is the bare-SSH host kind.
type Connector struct {
	HostName string
}

var _ host.Connector = (*Connector)(nil)

var ErrNotImplemented = errors.New("bare connector: not yet implemented (ADR 0008 Phase 2)")

func (c *Connector) Probe(_ context.Context) (host.Status, error) {
	return host.Status{
		Online:    false,
		LastError: ErrNotImplemented.Error(),
	}, nil
}

func (c *Connector) Inventory(_ context.Context) (host.Inventory, error) {
	return host.Inventory{}, ErrNotImplemented
}
