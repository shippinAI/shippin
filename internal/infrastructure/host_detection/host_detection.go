// Package host_detection finds out which kind of hypervisor answers at an address, before a person adding
// a server chooses one and enters its credentials. Staff who may add hosts run it; nothing is stored. Its
// actions are the other files in this folder; see docs/mcpl.md.
package host_detection

import (
	"context"

	"shippin.cloud/console/internal/hv"
)

// Query is the address to check: a host name or IP, host:port, or an https URL.
type Query struct {
	Address string `query:"address" json:"-"`
}

// Prober checks an address. The panel's is hv.Detect; a test may hand in another.
type Prober interface {
	Detect(ctx context.Context, address string) (hv.Detection, error)
}

// HV is the Prober that really connects, through hv.Detect.
type HV struct{}

func (HV) Detect(ctx context.Context, address string) (hv.Detection, error) {
	return hv.Detect(ctx, address)
}
