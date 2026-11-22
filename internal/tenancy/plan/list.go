package plan

import "shippin.cloud/console/internal/mcpl"

// List is shippin.tenancy.plan.list: GET /v1/operator/tenancy/plans. Each plan carries its services, so
// the whole plan x service matrix is one read.
var List = mcpl.Def[struct{}, mcpl.Items[Plan]]{
	Spec: mcpl.Spec{Summary: "List the plans, lowest rank first, each with the services it includes.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run: func(c *mcpl.Call, _ struct{}) (mcpl.Items[Plan], error) {
		ps, err := read(c, "")
		return mcpl.Items[Plan]{Items: ps}, err
	},
}
