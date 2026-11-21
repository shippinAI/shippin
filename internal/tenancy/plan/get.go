package plan

import "shippin.cloud/console/internal/mcpl"

// Get is shippin.tenancy.plan.get: GET /v1/operator/tenancy/plans/{planId}.
var Get = mcpl.Def[ByID, Plan]{
	Spec: mcpl.Spec{Summary: "Read one plan with the services it includes.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run:  func(c *mcpl.Call, in ByID) (Plan, error) { return one(c, in.ID) },
}
