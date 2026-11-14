// Package coverage is an organization's own coverage: the tier its plan includes, or the one staff
// moved it to. Members read it; prices stay with staff. Tiers themselves are in coverage_tier.
package coverage

import "shippin.cloud/console/internal/tenancy/coverage_tier"

// Own is the organization's coverage; Coverage is nil when its plan includes none.
type Own struct {
	Coverage *coverage_tier.CustomerView `json:"coverage"`
}
