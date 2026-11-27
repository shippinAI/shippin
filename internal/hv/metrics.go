package hv

import "context"

// HostMetrics are what a host reports about its own load at one moment, for the metrics sampler
// (OBS-01, docs/metrics.md). A nil field is one the provider did not report: it stays missing, never
// zero.
type HostMetrics struct {
	// Node is the host's own name as the provider knows it.
	Node string
	// CPUs is how many CPUs the host has; 0 when not reported.
	CPUs int
	// CPU is the busy fraction of all the host's CPUs, 0 to 1.
	CPU *float64
	// Load1 is the one-minute load average.
	Load1       *float64
	MemoryUsed  *int64
	MemoryTotal *int64
	// DiskUsed and DiskTotal are the host's root filesystem.
	DiskUsed  *int64
	DiskTotal *int64
	// NetInRate and NetOutRate are bytes per second the provider already averaged over its own
	// recent interval, so the sampler stores them as they are.
	NetInRate  *float64
	NetOutRate *float64
}

// HostMeter is a driver that can read its host's load.
type HostMeter interface {
	HostMetrics(ctx context.Context) (HostMetrics, error)
}

// GuestReading is one guest's resource readings at one moment. A nil field was not reported.
// Counters (CPUSeconds, NetIn, NetOut) only ever grow while the guest runs; the sampler turns two
// readings into a rate and treats a counter that went backwards as a restart.
type GuestReading struct {
	ID     string
	Node   string
	Status string
	// CPU is the busy fraction of the guest's own CPUs, 0 to 1, when the provider reports a fraction.
	CPU *float64
	// CPUSeconds is the guest's cumulative CPU time, when the provider reports a counter instead.
	CPUSeconds *float64
	// CPULimit is how many CPUs the guest may use, to turn CPUSeconds into a fraction. 0 means it may
	// use all of the host's.
	CPULimit    int
	MemoryUsed  *int64
	MemoryTotal *int64
	DiskUsed    *int64
	DiskTotal   *int64
	// NetIn and NetOut are cumulative bytes into and out of the guest, over all its interfaces but
	// loopback.
	NetIn  *int64
	NetOut *int64
}

// GuestMeter is a driver that can read every guest's resources in one call.
type GuestMeter interface {
	GuestReadings(ctx context.Context) ([]GuestReading, error)
}

// Ptr is a pointer to v, for the optional fields above.
func Ptr[T any](v T) *T { return &v }

// HostMonitorState is whether a server has the panel's host monitor, and whether it is running.
type HostMonitorState struct {
	Enabled bool `json:"enabled"`
	// Guest is the monitor's guest on the server; empty when there is none.
	Guest   string `json:"guest,omitempty"`
	Running bool   `json:"running"`
}

// HostMonitored is a driver whose own API cannot say how busy the host is now, and which reads that
// through a small guest it makes for the purpose when asked to. Nothing is made until then.
type HostMonitored interface {
	HostMonitor(ctx context.Context) (HostMonitorState, error)
	// EnableHostMonitor makes the monitor from the image the spec names and starts it.
	EnableHostMonitor(ctx context.Context, image GuestSpec) (HostMonitorState, error)
	DisableHostMonitor(ctx context.Context) error
}
