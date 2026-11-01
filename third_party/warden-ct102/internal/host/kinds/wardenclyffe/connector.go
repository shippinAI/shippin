// Package wardenclyffe implements host.Connector for the Wardenclyffe-fork panel.
//
// Wardenclyffe is our own fork of Proxmox VE — same open-source basis,
// rebranded panel, with our own divergent features layered on. Hosts of
// kind `wardenclyffe` route through this connector so the brand and
// fork-specific behavior surface in the UI.
//
// Phase 1 (this commit): stub. The connector registers itself but
// reports "not yet implemented".
//
// Phase 2: embeds the proxmox connector and proxies the Proxmox-API
// surface; only the brand label and any fork-divergent endpoints differ.
//
// Phase 3+: as the Wardenclyffe fork accrues features the proxmox API
// doesn't have, this package overrides or extends the embedded base.
package wardenclyffe

import (
	"context"
	"errors"

	"warden/internal/host"
)

func init() {
	host.Register(host.KindWardenclyffe, NewConnector)
}

// NewConnector constructs a Wardenclyffe-fork Connector for the given Host.
func NewConnector(h *host.Host) (host.Connector, error) {
	if h == nil {
		return nil, errors.New("wardenclyffe: nil host")
	}
	return &Connector{HostName: h.Name}, nil
}

// Connector is the Wardenclyffe-fork host kind.
//
// Phase 2: embed *proxmox.Connector and forward Probe/Inventory to it,
// overriding only what the fork diverges on. For now it stands alone as
// a stub.
type Connector struct {
	HostName string
}

var _ host.Connector = (*Connector)(nil)

// ErrNotImplemented is returned until Phase 2 wires the real connector.
var ErrNotImplemented = errors.New("wardenclyffe connector: not yet implemented (ADR 0008 Phase 2)")

func (c *Connector) Probe(_ context.Context) (host.Status, error) {
	return host.Status{
		Online:    false,
		LastError: ErrNotImplemented.Error(),
	}, nil
}

func (c *Connector) Inventory(_ context.Context) (host.Inventory, error) {
	return host.Inventory{}, ErrNotImplemented
}
