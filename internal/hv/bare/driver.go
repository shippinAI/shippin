// Package bare is a server with no hypervisor API. It never reports a fake online state.
package bare

import (
	"context"
	"time"

	"shippin.cloud/console/internal/hv"
)

// Driver is the bare host.
type Driver struct{ Now func() time.Time }

func (d Driver) Kind() string { return "bare" }

func (d Driver) Health(context.Context) (hv.Health, error) {
	now := time.Now()
	if d.Now != nil {
		now = d.Now()
	}
	return hv.Health{Online: false, Error: hv.ErrNoHypervisor.Error(), Checked: now}, hv.ErrNoHypervisor
}

func (d Driver) Guests(context.Context) ([]hv.Guest, error) { return nil, hv.ErrNoHypervisor }
func (d Driver) Guest(context.Context, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrNoHypervisor
}
func (d Driver) Start(context.Context, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrNoHypervisor
}
func (d Driver) Stop(context.Context, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrNoHypervisor
}
func (d Driver) Shutdown(context.Context, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrNoHypervisor
}
func (d Driver) Reboot(context.Context, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrNoHypervisor
}
func (d Driver) Snapshot(context.Context, string, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrNoHypervisor
}
func (d Driver) Storage(context.Context) ([]hv.Volume, error) { return nil, hv.ErrNoHypervisor }
func (d Driver) Networks(context.Context) ([]hv.Network, error) {
	return nil, hv.ErrNoHypervisor
}
