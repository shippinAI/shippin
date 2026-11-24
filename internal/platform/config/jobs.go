package config

import (
	"errors"
	"time"
)

// Jobs configures the durable job runner (M1). Disabled, the console keeps the in-process path.
type Jobs struct {
	Enabled          bool `json:"enabled"`
	Workers          int  `json:"workers"`
	MaxAttempts      int  `json:"max_attempts"`
	LeaseSeconds     int  `json:"lease_seconds"`
	HeartbeatSeconds int  `json:"heartbeat_seconds"`
	PollSeconds      int  `json:"poll_seconds"`
	BackoffSeconds   int  `json:"backoff_seconds"`
	BackoffMaxSecs   int  `json:"backoff_max_seconds"`
	ReconcileSeconds int  `json:"reconcile_seconds"`
	DriftSeconds     int  `json:"drift_seconds"`
	KeepDays         int  `json:"keep_days"`
	IdempotencyHours int  `json:"idempotency_hours"`
	// ProvisionPlannedBoundaries lets the reconciler create tenant VMs for planned boundaries.
	ProvisionPlannedBoundaries bool `json:"provision_planned_boundaries"`
}

// Sec converts a seconds setting to a duration.
func Sec(n int) time.Duration { return time.Duration(n) * time.Second }

// Check fails closed when the runner is enabled with missing or inconsistent timings.
func (j Jobs) Check() error {
	if !j.Enabled {
		return nil
	}
	switch {
	case j.Workers < 1 || j.MaxAttempts < 1:
		return errors.New("jobs.workers and jobs.max_attempts must be at least 1")
	case j.LeaseSeconds < 1 || j.HeartbeatSeconds < 1 || j.HeartbeatSeconds*2 > j.LeaseSeconds:
		return errors.New("jobs.lease_seconds must be at least twice jobs.heartbeat_seconds")
	case j.PollSeconds < 1 || j.BackoffSeconds < 1 || j.BackoffMaxSecs < j.BackoffSeconds:
		return errors.New("jobs.poll_seconds, backoff_seconds and backoff_max_seconds must be set")
	case j.ReconcileSeconds < 1 || j.DriftSeconds < 1 || j.KeepDays < 1 || j.IdempotencyHours < 1:
		return errors.New("jobs.reconcile_seconds, drift_seconds, keep_days and idempotency_hours must be set")
	}
	return nil
}
