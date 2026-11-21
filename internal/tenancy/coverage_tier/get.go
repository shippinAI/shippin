package coverage_tier

import "shippin.cloud/console/internal/mcpl"

var Get = mcpl.Def[ByID, Tier]{
	Spec: mcpl.Spec{Summary: "Read one coverage tier.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run:  func(c *mcpl.Call, in ByID) (Tier, error) { return one(c, in.ID) },
}
