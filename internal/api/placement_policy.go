package api

import (
	"shippin.cloud/console/internal/ext"
	"shippin.cloud/console/internal/ext/placement"
)

// placement is the placement policy the panel uses: the registered module's, or today's server
// assignments.
func (a *API) placement() placement.Policy {
	if p := ext.Placement(); p != nil {
		return p
	}
	return placement.Assigned{}
}
