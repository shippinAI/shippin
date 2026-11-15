package runner

import (
	"errors"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/settings"
)

// Settings are where build runner machines run (docs/runners.md): the provider's own capacity. Which
// organizations may use runners, their sizes and limits are plan settings.
var Settings = settings.Section{
	Key:     "runners",
	Title:   "Build runners",
	Summary: "Where customers' GitHub Actions jobs run. Which plans include runners, their sizes and limits are set on Plans & tiers.",
	Fields: []settings.Field{
		{Name: "enabled", Label: "Run build runners", Kind: settings.Bool,
			Help: "Off, no runner machine starts and customers' queued jobs wait. Needs the job queue on."},
		{Name: "fleet", Label: "Fleet", Kind: settings.Text, Optional: true, Source: "fleets",
			Help: "The fleet whose hosts run runner machines. Each host needs an Incus API attached."},
		{Name: "storage", Label: "Storage pool", Kind: settings.Text, Optional: true,
			Help: "The storage pool on those hosts for a runner's disk, sized from its runner size. Empty keeps the host's default disk."},
		{Name: "job_limit_minutes", Label: "Longest a runner machine may live (minutes)", Kind: settings.Integer, Min: 0, Max: 10080,
			Help: "A machine older than this is removed whatever its job says, so a stuck one never holds capacity. 5 to 10080 while runners are on."},
	},
	Seed: func(c *config.Config) any { return c.Runners },
	Check: func(v settings.Values, c settings.Checking) error {
		r, err := settings.As[config.Runners](v)
		if err != nil {
			return err
		}
		if err := r.Check(); err != nil {
			return err
		}
		if on, _ := c.Section("jobs")["enabled"].(bool); r.Enabled && !on {
			return errors.New("turn on the job queue first: starting and removing runner machines are durable jobs")
		}
		return nil
	},
}
