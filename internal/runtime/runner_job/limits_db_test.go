package runner_job_test

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/ext/limits"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/runtime/runner_job"
)

// probeLimits gives Globex, whose plan has no runners, the 8x size; it refuses Acme with its own reason.
type probeLimits struct{ asked []string }

func (p *probeLimits) Runners(_ context.Context, q limits.Querier, org, workspace string) (limits.Runners, error) {
	if q == nil {
		panic("no transaction")
	}
	p.asked = append(p.asked, org+"/"+workspace)
	if org == globex {
		return limits.Runners{Enabled: true, Concurrent: 1, Sizes: []string{"8x"}}, nil
	}
	return limits.Runners{Why: "The probe module has paused this organization's builds."}, nil
}

// A limits policy decides what a job may use: its sizes and its reasons, not the organization's plan.
func TestALimitsPolicyDecidesTheJob(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.runtime.runner_job.", mcpltest.As{Actor: "ada", Tenant: acme})
	if _, err := h.Pool.Exec(t.Context(), fixture); err != nil {
		t.Fatal(err)
	}
	p := &probeLimits{}
	receive := func(id, inst, repo int64, size string) runner_job.Outcome {
		t.Helper()
		out, err := runner_job.ReceiveUnder(t.Context(), h.Pool, p, runner_job.Webhook{Action: "queued", InstallationID: inst,
			RepositoryID: repo, Repository: "x/y", JobID: id, RunID: 9, Name: "build", Labels: []string{"shippin-" + size}})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	if out := receive(1, 22, 201, "8x"); out.Status != runner_job.Queued || out.Reason != "" {
		t.Fatalf("the policy's size must be allowed though the plan has no runners: %+v", out)
	}
	if out := receive(2, 22, 201, "2x"); out.Status != runner_job.Refused || !contains(out.Reason, "does not include runner size 2x") {
		t.Fatalf("a size the policy does not give must be refused: %+v", out)
	}
	if out := receive(3, 11, 101, "2x"); out.Status != runner_job.Refused || out.Reason != "The probe module has paused this organization's builds." {
		t.Fatalf("the policy's reason must be the one shown: %+v", out)
	}
	if len(p.asked) != 3 || p.asked[0] != globex+"/"+wsB || p.asked[2] != acme+"/"+wsA {
		t.Errorf("the policy was asked %v", p.asked)
	}
}
