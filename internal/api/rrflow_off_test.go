package api

import (
	"context"
	"errors"
	"testing"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/managed"
)

// A queued service.provision job (from migration 026 or a newer release) never runs on a release
// where rrflow is off or the tenant is outside the rollout (GC-20261019-PB ruling e).
func TestServiceJobHonorsRollout(t *testing.T) {
	tn := "aaaaaaaa-0000-7000-8000-000000000001"
	for name, rr := range map[string]config.ServiceToggle{
		"off":         {Enabled: false},
		"other":       {Enabled: true, Tenants: []string{"aaaaaaaa-0000-7000-8000-000000000002"}},
		"listed-none": {Enabled: true, ListedOnly: true},
	} {
		a := &API{cfg: &config.Config{}, svcs: &managed.Store{}}
		a.cfg.Services.RRFlow = rr
		_, err := a.runServiceJob(context.Background(), &jobs.Job{TenantID: tn, TargetID: "x"}, func(string) {})
		if !errors.Is(err, errRRFlowOff) || !jobs.IsPermanent(err) {
			t.Errorf("%s: job ran: %v", name, err)
		}
	}
}
