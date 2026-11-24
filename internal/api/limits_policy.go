package api

import (
	"shippin.cloud/console/internal/ext"
	"shippin.cloud/console/internal/ext/limits"
)

// limits is the limits policy the panel uses: the registered module's, or today's plan limits.
func (a *API) limits() limits.Policy {
	if p := ext.Limits(); p != nil {
		return p
	}
	return limits.FromPlan{}
}
