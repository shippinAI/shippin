package api

import (
	"context"
	"log"
	"sync"
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/settings"
)

// settingUpdated is the event of a saved settings section.
const settingUpdated = "shippin.platform.setting.updated"

// jobQueue is the durable job runner and the reconciler the job queue section has running.
type jobQueue struct {
	mu      sync.Mutex
	on      bool
	timings jobTimings
	runner  *jobs.Runner
	// stopReconciler closes to end the reconciler after the pass it is in.
	stopReconciler chan struct{}
	reported       bool
}

// jobTimings are the parts of the job queue section the runner and reconciler are started with; a
// change to any of them restarts both. The rest (tries, idempotency, retention) is read where used.
type jobTimings struct {
	Workers, Lease, Heartbeat, Poll, Backoff, BackoffMax, Reconcile, Drift int
}

func timingsOf(j config.Jobs) jobTimings {
	return jobTimings{j.Workers, j.LeaseSeconds, j.HeartbeatSeconds, j.PollSeconds, j.BackoffSeconds, j.BackoffMaxSecs, j.ReconcileSeconds, j.DriftSeconds}
}

// jobRunner is the running job runner, or nil while the job queue is off: then handlers take the
// in-process path.
func (a *API) jobRunner() *jobs.Runner { return a.runner.Load() }

// StartJobs runs the durable job runner and the reconciler while the job queue section is on, and
// follows the section until ctx ends (CFG-01): turning it on starts them, turning it off stops them
// after the jobs they hold finish, and changing a timing starts new ones while the old ones finish
// theirs. A save in this panel is applied at once; one in another panel within settings.Fresh. Call
// it before serving: handlers read the runner to choose the job path over the in-process path.
func (a *API) StartJobs(ctx context.Context) {
	a.applyJobs(ctx)
	kick := make(chan struct{}, 1)
	a.events.On(settingUpdated, func(context.Context, mcpl.Event) {
		// After the commit: drop what may have been read while the save was open, then look again.
		if a.settings != nil {
			a.settings.Invalidate()
		}
		select {
		case kick <- struct{}{}:
		default:
		}
	})
	go func() {
		t := time.NewTicker(settings.Fresh)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-kick:
			}
			a.applyJobs(ctx)
		}
	}()
}

// applyJobs makes what runs match the job queue section. A section that can't be read changes nothing.
func (a *API) applyJobs(ctx context.Context) {
	j, ok := a.jobsSection(ctx)
	if !ok || ctx.Err() != nil {
		return
	}
	q := &a.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	want := timingsOf(j)
	switch {
	case !j.Enabled && !q.on:
		if !q.reported {
			log.Printf("jobs: disabled (in-process provisioning)")
		}
	case !j.Enabled:
		q.stop()
		a.runner.Store(nil)
		log.Printf("jobs: turned off; jobs in progress finish, new work runs in the request")
	case q.on && q.timings == want:
	default:
		restart := q.on
		q.stop()
		r := jobs.NewRunner(a.jobs, jobs.Settings{Workers: j.Workers,
			Lease: config.Sec(j.LeaseSeconds), Heartbeat: config.Sec(j.HeartbeatSeconds), Poll: config.Sec(j.PollSeconds),
			BackoffBase: config.Sec(j.BackoffSeconds), BackoffMax: config.Sec(j.BackoffMaxSecs)})
		a.registerJobs(r)
		stop := make(chan struct{})
		q.on, q.timings, q.runner, q.stopReconciler = true, want, r, stop
		a.runner.Store(r)
		go r.Run(ctx)
		go a.runReconciler(ctx, stop, j)
		if restart {
			log.Printf("jobs: timings changed; %d workers (the previous ones finish the jobs they hold)", j.Workers)
		} else {
			log.Printf("jobs: %d workers", j.Workers)
		}
	}
	q.reported = true
}

// stop ends the runner and reconciler q has running: they take no more work and finish what they hold.
func (q *jobQueue) stop() {
	if q.runner != nil {
		q.runner.Stop()
	}
	if q.stopReconciler != nil {
		close(q.stopReconciler)
	}
	q.on, q.runner, q.stopReconciler = false, nil, nil
}

// registerJobs wires every job kind to its handler.
func (a *API) registerJobs(r *jobs.Runner) {
	r.Register(jobs.KindWorkspaceAction, a.runWorkspaceActionJob)
	r.Register(jobs.KindWorkspaceProvision, a.runWorkspaceJob)
	r.Register(jobs.KindTenantProvision, a.runTenantJob)
	r.Register(jobs.KindBoundaryProvision, a.runBoundaryJob)
	r.Register(jobs.KindAIActivate, a.runAIJob)
	r.Register(jobs.KindProjectImport, a.runImportJob)
	r.Register(jobs.KindProjectPrompt, a.runPromptJob)
	r.Register(jobs.KindAgentStart, a.runAgentStartJob)
	r.Register(jobs.KindServiceProvision, a.runServiceJob)
	r.Register(jobs.KindDeploymentReconcile, a.runDeploymentReconcileJob)
	r.Register(jobs.KindRunnerProvision, a.runRunnerProvision)
	r.Register(jobs.KindRunnerRemove, a.runRunnerRemove)
}
