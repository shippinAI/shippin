package config

import (
	"errors"
	"regexp"

	"github.com/google/uuid"
)

// Runners configures where build runner machines run (RUN-01, docs/runners.md). Which organizations
// may use them, which sizes and how many at once are plan settings in the panel; this is only the
// provider's own capacity. Disabled, nothing starts a runner machine and queued jobs keep waiting.
// It needs jobs.enabled: starting and removing machines are durable jobs.
type Runners struct {
	Enabled bool `json:"enabled"`
	// Fleet is the infrastructure fleet (Infrastructure > Fleets in the panel) whose hosts run runner
	// machines. Staff add or remove hosts there; each must have an Incus API attached.
	Fleet string `json:"fleet"`
	// Storage is the storage pool on those hosts for a machine's root disk, sized from its runner
	// size. Empty keeps the host profile's root disk and size.
	Storage string `json:"storage"`
	// JobLimitMinutes is how long a machine may live. One older than this is removed whatever its
	// job says, so a hung or forgotten machine never holds capacity for good.
	JobLimitMinutes int `json:"job_limit_minutes"`
}

var poolName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// Check fails closed when runners are enabled without a fleet or a time limit.
func (r Runners) Check() error {
	if !r.Enabled {
		return nil
	}
	switch {
	case uuid.Validate(r.Fleet) != nil:
		return errors.New("runners.fleet must be the id of the fleet whose hosts run runner machines")
	case r.Storage != "" && !poolName.MatchString(r.Storage):
		return errors.New("runners.storage must be a storage pool name")
	case r.JobLimitMinutes < 5 || r.JobLimitMinutes > 7*24*60:
		return errors.New("runners.job_limit_minutes must be between 5 and 10080")
	}
	return nil
}
