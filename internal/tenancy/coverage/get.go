package coverage

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/coverage_tier"
)

var Get = mcpl.Def[struct{}, Own]{
	Spec: mcpl.Spec{Summary: "Read what your organization's coverage includes: support hours, response targets and services.", Access: mcpl.Member, Singleton: true},
	Run: func(c *mcpl.Call, _ struct{}) (Own, error) {
		v, err := coverage_tier.ForTenant(c.Ctx, c.DB, c.Tenant)
		return Own{Coverage: v}, err
	},
}
