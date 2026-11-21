package effective_config

import (
	"errors"

	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/customer"
)

// Get is shippin.tenancy.customer.effective_config.get: GET /v1/operator/tenancy/customers/{customerId}/effective-config.
var Get = mcpl.Def[Query, catalog.EffectiveConfig]{
	Spec: mcpl.Spec{Summary: "Show the settings and services a customer ends up with once every layer is merged, and why.", Access: mcpl.Staff, Perm: "config.effective.read", Singleton: true},
	Run: func(c *mcpl.Call, in Query) (catalog.EffectiveConfig, error) {
		eff, err := catalog.EffectiveFor(c.Ctx, c.DB, in.CustomerID, in.Workspace)
		if errors.Is(err, catalog.ErrNotFound) {
			return catalog.EffectiveConfig{}, customer.NotFound()
		}
		if err != nil {
			return catalog.EffectiveConfig{}, err
		}
		return *eff, nil
	},
}
