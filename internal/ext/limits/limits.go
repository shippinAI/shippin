// Package limits is how much an organization may use when work starts (docs/editions.md, the limits
// policy). The panel asks the registered module's policy (ext.Module.Limits) if one is set, and
// FromPlan otherwise. It imports nothing from the panel's features, so the work it limits can use it.
package limits

import (
	"context"
	"encoding/json"
	"errors"

	"shippin.cloud/console/internal/services/catalog"
)

// Querier is the caller's database handle, usually its transaction, so a limit is read in the same
// transaction as the work it limits.
type Querier = catalog.Querier

// Runners is what an organization's build jobs may use.
type Runners struct {
	// Enabled says the organization may use build runners at all.
	Enabled bool
	// Concurrent is how many runner machines it may have at once.
	Concurrent int
	// Sizes are the runner sizes its jobs may ask for.
	Sizes []string
	// MonthlyMinutes is how many runner minutes it may use each calendar month (0 for no limit).
	MonthlyMinutes float64
	// Why says, in words for people, why the organization has no runners ("" when Enabled).
	Why string
}

// Policy says how much an organization may use.
type Policy interface {
	// Runners is what org's build jobs may use, in workspace ("" for the organization as a whole).
	Runners(ctx context.Context, q Querier, org, workspace string) (Runners, error)
}

// FromPlan is today's policy, until plans move to the enterprise module (roadmap S3): the limits are
// the svc.runners.* services of the organization's effective plan configuration.
type FromPlan struct{}

func (FromPlan) Runners(ctx context.Context, q Querier, org, workspace string) (Runners, error) {
	eff, err := catalog.EffectiveFor(ctx, q, org, workspace)
	if errors.Is(err, catalog.ErrNotFound) {
		return Runners{Why: "This organization has no plan, so it has no build runners."}, nil
	}
	if err != nil {
		return Runners{}, err
	}
	var out Runners
	var concurrent float64
	for _, s := range eff.Services {
		switch s.Key {
		case "svc.runners.enabled":
			_ = json.Unmarshal(s.Value, &out.Enabled)
		case "svc.runners.concurrent":
			_ = json.Unmarshal(s.Value, &concurrent)
		case "svc.runners.sizes":
			_ = json.Unmarshal(s.Value, &out.Sizes)
		case "svc.runners.minutes":
			_ = json.Unmarshal(s.Value, &out.MonthlyMinutes)
		}
	}
	if !out.Enabled {
		out.Why = "Your plan does not include build runners."
	}
	if concurrent >= 1 {
		out.Concurrent = int(concurrent)
	}
	if out.MonthlyMinutes < 0 {
		out.MonthlyMinutes = 0
	}
	return out, nil
}
