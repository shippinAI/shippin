// Connector dispatch + registry helpers.
//
// The `Connector` interface itself lives in host.go alongside the data
// types it operates on. This file holds the dispatch from a Host's Kind
// to a concrete Connector constructor — the only place feature code
// needs to know which kinds exist.
//
// Phase 2 of ADR 0008 fills in the constructor functions.

package host

import (
	"context"
	"errors"
	"fmt"
)

// ErrUnknownKind is returned when a Host's Kind doesn't match any registered Connector constructor.
var ErrUnknownKind = errors.New("host: unknown kind")

// connectorFactory builds a Connector for a single Host.
type connectorFactory func(*Host) (Connector, error)

// kindRegistry maps Kind -> constructor. Populated at process init by each
// kind package's init() (or by NewConnectorFor's lazy import path during
// Phase 2). Read-only after init; do not mutate at runtime.
var kindRegistry = map[Kind]connectorFactory{}

// Register installs a Connector constructor for a Kind. Each kind
// package's init() calls this exactly once.
func Register(k Kind, fn connectorFactory) {
	if !k.IsKnown() {
		panic(fmt.Sprintf("host.Register: unknown kind %q", k))
	}
	if _, exists := kindRegistry[k]; exists {
		panic(fmt.Sprintf("host.Register: kind %q already registered", k))
	}
	kindRegistry[k] = fn
}

// NewConnectorFor builds a Connector for h based on h.Kind.
// Returns ErrUnknownKind if no constructor is registered.
func NewConnectorFor(h *Host) (Connector, error) {
	fn, ok := kindRegistry[h.Kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q (host %q)", ErrUnknownKind, h.Kind, h.Name)
	}
	return fn(h)
}

// noopConnector is returned by registry tests / dry-run paths. It always
// reports offline with a clear error so consumers see the unconfigured state.
type noopConnector struct{ host string }

// NoopConnector returns a Connector that always reports offline.
// Useful for tests and dev-mode hosts that have no real backend.
func NoopConnector(hostName string) Connector {
	return &noopConnector{host: hostName}
}

func (c *noopConnector) Probe(_ context.Context) (Status, error) {
	return Status{Online: false, LastError: "noop connector"}, nil
}

func (c *noopConnector) Inventory(_ context.Context) (Inventory, error) {
	return Inventory{}, fmt.Errorf("noop connector: inventory not available for host %q", c.host)
}
