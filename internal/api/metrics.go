package api

import (
	"context"

	"shippin.cloud/console/internal/clickhouse"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/metric"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/settings"
)

// samplerLock is the advisory lock held by the one panel process that records metrics ("SHIPMET").
const samplerLock int64 = 0x53_4849_504d_4554

// RunMetrics records every attached host's and guest's metrics in ClickHouse until ctx ends, on the
// schedule the metrics section sets (OBS-01, docs/metrics.md). Of several panel processes only the one
// holding samplerLock samples; the others stand by and take over when it goes. A settings save makes it
// look at its section at once.
func (a *API) RunMetrics(ctx context.Context) {
	if a.db == nil || a.settings == nil {
		return
	}
	s := &metric.Sampler{
		Targets:    a.metricTargets,
		ClickHouse: a.ch.Load,
		Config: func(ctx context.Context) metric.Config {
			c, _ := settings.Get[metric.Config](ctx, a.settings, metric.Settings.Key)
			return c
		},
		Lead: &metric.PGLeader{Pool: a.db.Pool, Key: samplerLock},
	}
	a.sampler.Store(s)
	kick := make(chan struct{}, 1)
	a.events.On(settingUpdated, func(context.Context, mcpl.Event) {
		a.settings.Invalidate()
		select {
		case kick <- struct{}{}:
		default:
		}
	})
	s.Run(ctx, kick)
}

// metricTargets are the attached hosts, each opened with its own credentials on each tick.
func (a *API) metricTargets(ctx context.Context) ([]metric.Target, error) {
	eps, err := a.hvDir().Endpoints(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]metric.Target, 0, len(eps))
	for _, ep := range eps {
		out = append(out, metric.Target{ServerID: ep.ID, Open: func() (hv.Driver, error) { return a.openDriver(ep) }})
	}
	return out, nil
}

// metricsPort is ports.Metrics on this panel's ClickHouse and sampler.
type metricsPort struct{ a *API }

func (p metricsPort) ClickHouse() *clickhouse.Client { return p.a.ch.Load() }

func (p metricsPort) Sampling() ports.Sampling {
	if s := p.a.sampler.Load(); s != nil {
		return s.Status()
	}
	return ports.Sampling{State: "idle", Reason: "This panel process does not run the metrics sampler."}
}
