package bare

import (
	"context"
	"errors"
	"testing"

	"shippin.cloud/console/internal/hv"
)

func TestBareNeverLooksOnline(t *testing.T) {
	var d Driver
	h, err := d.Health(context.Background())
	if !errors.Is(err, hv.ErrNoHypervisor) || h.Online {
		t.Fatalf("%+v %v", h, err)
	}
	if _, err := d.Start(context.Background(), "1"); !errors.Is(err, hv.ErrNoHypervisor) {
		t.Fatal(err)
	}
}
