package hostd

import (
	"context"
	"fmt"
	"regexp"
)

// Follow Incus' documented instance-name contract, failing closed on other forms.
// https://linuxcontainers.org/incus/docs/main/reference/instance_properties/
var utilityInventoryName = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// utilityExists distinguishes a successful inventory with no match from an
// Incus/transport failure. Ownership is verified before mounting or stopping.
func utilityExists(ctx context.Context, at place, account string) (bool, error) {
	var instances []*struct {
		Name   string            `json:"name"`
		Config map[string]string `json:"config"`
	}
	if err := at.query(ctx, "/1.0/instances?recursion=1", &instances); err != nil {
		return false, err
	}
	if instances == nil {
		return false, fmt.Errorf("utility inventory unavailable")
	}
	found := false
	for _, inst := range instances {
		if inst == nil || !utilityInventoryName.MatchString(inst.Name) || inst.Config == nil {
			return false, fmt.Errorf("malformed utility inventory entry")
		}
		if inst.Name != UtilityInstance(account) {
			continue
		}
		if found || inst.Config["user.shippin.utility"] != account || inst.Config["user.shippin.tenant"] != at.t {
			return false, fmt.Errorf("utility instance ownership mismatch")
		}
		found = true
	}
	return found, nil
}
