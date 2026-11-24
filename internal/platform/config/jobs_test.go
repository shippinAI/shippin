package config

import "testing"

func TestJobsCheck(t *testing.T) {
	ok := Jobs{Enabled: true, Workers: 4, MaxAttempts: 5, LeaseSeconds: 60, HeartbeatSeconds: 20, PollSeconds: 2,
		BackoffSeconds: 10, BackoffMaxSecs: 300, ReconcileSeconds: 60, DriftSeconds: 600, KeepDays: 30, IdempotencyHours: 24}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	if err := (Jobs{}).Check(); err != nil {
		t.Fatalf("disabled jobs need no settings: %v", err)
	}
	bad := ok
	bad.HeartbeatSeconds = 40
	if bad.Check() == nil {
		t.Fatal("a heartbeat slower than half the lease must be refused")
	}
	bad = ok
	bad.Workers = 0
	if bad.Check() == nil {
		t.Fatal("zero workers must be refused")
	}
}
