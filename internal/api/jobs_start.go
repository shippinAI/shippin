package api

import (
	"context"
	"log"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/jobs"
)

// StartJobs starts the durable job runner and the reconciler when config jobs.enabled is true.
// Call it before serving: handlers read a.runner to choose the job path over the goroutine path.
func (a *API) StartJobs(ctx context.Context) {
	j := a.cfg.Jobs
	if !j.Enabled {
		log.Printf("jobs: disabled (in-process provisioning)")
		return
	}
	a.runner = jobs.NewRunner(a.jobs, jobs.Settings{Workers: j.Workers,
		Lease: config.Sec(j.LeaseSeconds), Heartbeat: config.Sec(j.HeartbeatSeconds), Poll: config.Sec(j.PollSeconds),
		BackoffBase: config.Sec(j.BackoffSeconds), BackoffMax: config.Sec(j.BackoffMaxSecs)})
	a.registerJobs(a.runner)
	go a.runner.Run(ctx)
	go a.runReconciler(ctx)
	log.Printf("jobs: %d workers", j.Workers)
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
}
