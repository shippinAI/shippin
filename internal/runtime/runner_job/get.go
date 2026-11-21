package runner_job

import "shippin.cloud/console/internal/mcpl"

// Get is shippin.runtime.runner_job.get: GET /v1/tenants/{tenantId}/runtime/runner-jobs/{runnerJobId}.
var Get = mcpl.Def[ByID, Job]{
	Spec: mcpl.Spec{Summary: "Read one runner job: its repository, size, state and timing.", Access: mcpl.Member},
	Run:  func(c *mcpl.Call, in ByID) (Job, error) { return one(c, in.ID) },
}
