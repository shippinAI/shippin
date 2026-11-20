package runner

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/limits"
)

type fixedLimits limits.Runners

func (f fixedLimits) Runners(context.Context, limits.Querier, string, string) (limits.Runners, error) {
	return limits.Runners(f), nil
}

// How many machines an organization may have at once is the policy's, and none when it may not use
// runners at all.
func TestLimitIsThePolicys(t *testing.T) {
	for _, c := range []struct {
		r    limits.Runners
		want int
	}{
		{limits.Runners{Enabled: true, Concurrent: 3}, 3},
		{limits.Runners{Enabled: false, Concurrent: 3}, 0},
		{limits.Runners{Enabled: true}, 0},
	} {
		if got, err := Limit(context.Background(), fixedLimits(c.r), nil, "org"); err != nil || got != c.want {
			t.Errorf("%+v: %d %v, want %d", c.r, got, err, c.want)
		}
	}
	if _, ok := (Deps{}).limits().(limits.FromPlan); !ok {
		t.Error("without a policy the limits are the plan's")
	}
	if _, ok := (Deps{Limits: fixedLimits{}}).limits().(fixedLimits); !ok {
		t.Error("a policy in Deps must be the one used")
	}
}
