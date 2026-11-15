// Package job_queue is the settings of the durable job queue (internal/jobs): whether it runs and its
// timings. Starting and removing machines, provisioning and deployments use it.
package job_queue

import (
	"errors"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/settings"
)

// Settings is the job queue. Turning it on starts the workers at once; changing a timing restarts them
// after the jobs they hold finish.
var Settings = settings.Section{
	Key:     "jobs",
	Title:   "Job queue",
	Summary: "Background work that must happen exactly once, even across restarts: provisioning, deployments, runner machines.",
	Fields: []settings.Field{
		{Name: "enabled", Label: "Run the job queue", Kind: settings.Bool,
			Help: "Off, provisioning runs inside the request instead, and build runners can't run."},
		{Name: "workers", Label: "Jobs at once", Kind: settings.Integer, Min: 1, Max: 64},
		{Name: "max_attempts", Label: "Tries before a job fails", Kind: settings.Integer, Min: 1, Max: 50},
		{Name: "lease_seconds", Label: "Lease (seconds)", Kind: settings.Integer, Min: 1, Max: 3600,
			Help: "How long a worker holds a job between heartbeats before another may take it over. At least twice the heartbeat."},
		{Name: "heartbeat_seconds", Label: "Heartbeat (seconds)", Kind: settings.Integer, Min: 1, Max: 1800},
		{Name: "poll_seconds", Label: "Check for new jobs every (seconds)", Kind: settings.Integer, Min: 1, Max: 300},
		{Name: "backoff_seconds", Label: "First retry after (seconds)", Kind: settings.Integer, Min: 1, Max: 3600},
		{Name: "backoff_max_seconds", Label: "Longest wait between retries (seconds)", Kind: settings.Integer, Min: 1, Max: 86400},
		{Name: "reconcile_seconds", Label: "Reconcile every (seconds)", Kind: settings.Integer, Min: 1, Max: 3600,
			Help: "How often the panel compares what should exist with what does, and queues the difference."},
		{Name: "drift_seconds", Label: "Drift check every (seconds)", Kind: settings.Integer, Min: 1, Max: 86400},
		{Name: "keep_days", Label: "Keep finished jobs (days)", Kind: settings.Integer, Min: 1, Max: 3650},
		{Name: "idempotency_hours", Label: "Remember repeated requests (hours)", Kind: settings.Integer, Min: 1, Max: 720},
		{Name: "provision_planned_boundaries", Label: "Create planned tenant machines", Kind: settings.Bool,
			Help: "Lets the reconciler create the virtual machines planned for organizations that have none yet."},
	},
	Seed: func(c *config.Config) any { return c.Jobs },
	Check: func(v settings.Values, c settings.Checking) error {
		j, err := settings.As[config.Jobs](v)
		if err != nil {
			return err
		}
		if err := j.Check(); err != nil {
			return err
		}
		if on, _ := c.Section("runners")["enabled"].(bool); on && !j.Enabled {
			return errors.New("turn off build runners first: they need the job queue")
		}
		return nil
	},
}
