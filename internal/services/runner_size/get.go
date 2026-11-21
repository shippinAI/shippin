package runner_size

import "shippin.cloud/console/internal/mcpl"

// Get is shippin.services.runner_size.get: GET /v1/operator/services/runner-sizes/{runnerSizeId}.
var Get = mcpl.Def[ByID, Size]{
	Spec: mcpl.Spec{Summary: "Read one runner size.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run:  func(c *mcpl.Call, in ByID) (Size, error) { return one(c, in.ID) },
}
