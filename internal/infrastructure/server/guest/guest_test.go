package guest

import (
	"testing"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/mcpl"
)

func status(err error) int {
	if e, ok := err.(*mcpl.Error); ok {
		return e.Status
	}
	return 0
}

func TestMarkSelfFlagsOnlyThePanel(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "Panel-VM")
	got := MarkSelf([]hv.Guest{{ID: "100", Name: "panel-vm"}, {ID: "101", Name: "other"}})
	if !got[0].Self || got[1].Self {
		t.Fatalf("%+v", got)
	}
}

// The panel's own guest is changed only with a confirmation and the permission, and starting it or
// changing another guest is never held back.
func TestGuardSelf(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "panel-vm")
	self, other := hv.Guest{ID: "100", Name: "panel-vm"}, hv.Guest{ID: "101", Name: "other"}
	for _, c := range []struct {
		name           string
		g              hv.Guest
		verb           string
		confirmed, may bool
		want           int
	}{
		{"unconfirmed", self, "stop", false, true, 409},
		{"confirmed without the permission", self, "stop", true, false, 403},
		{"confirmed with it", self, "stop", true, true, 0},
		{"delete unconfirmed", self, "delete", false, true, 409},
		{"start", self, "start", false, false, 0},
		{"another guest", other, "stop", false, false, 0},
	} {
		if got := status(GuardSelf(c.g, c.verb, c.confirmed, c.may)); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// A new guest names what its server's kind takes and nothing it does not.
func TestPlacement(t *testing.T) {
	sized := []string{"memory", "cores"}
	placed := []string{"memory", "cores", "disk", "storage", "network"}
	for _, c := range []struct {
		name   string
		in     CreateInput
		fields []string
		ok     bool
	}{
		{"nothing to place", CreateInput{}, sized, true},
		{"storage where none is taken", CreateInput{Storage: "local"}, sized, false},
		{"network where none is taken", CreateInput{Network: "br0"}, sized, false},
		{"everything chosen", CreateInput{Storage: "local", Network: "br0", Disk: 8}, placed, true},
		{"no storage", CreateInput{Network: "br0", Disk: 8}, placed, false},
		{"no network", CreateInput{Storage: "local", Disk: 8}, placed, false},
		{"no disk", CreateInput{Storage: "local", Network: "br0"}, placed, false},
	} {
		if err := placement(c.in, c.fields); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
