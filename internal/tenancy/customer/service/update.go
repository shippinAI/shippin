package service

import (
	"encoding/json"

	"shippin.cloud/console/internal/mcpl"
)

// OverrideInput is the customer's settings for the service, replacing any it had. Enabled left out or
// null follows the plan; overrides are the service's settings by name.
type OverrideInput struct {
	ByID
	Enabled   *bool                      `json:"enabled,omitempty"`
	Overrides map[string]json.RawMessage `json:"overrides,omitempty"`
}

// Update is shippin.tenancy.customer.service.update: PUT /v1/operator/tenancy/customers/{customerId}/services/{serviceId}.
// It creates the customer's settings for the service, or replaces them.
var Update = mcpl.Def[OverrideInput, Override]{
	Spec: mcpl.Spec{Summary: "Turn a catalog service on or off for one customer, or override its settings.", Access: mcpl.Staff, Perm: "customers.services.write"},
	Run: func(c *mcpl.Call, in OverrideInput) (Override, error) {
		if err := in.must(c); err != nil {
			return Override{}, err
		}
		var known bool
		if err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM catalog_services WHERE id=$1)`, in.ID).Scan(&known); err != nil {
			return Override{}, err
		}
		if !known {
			return Override{}, mcpl.NotFound("There is no service " + in.ID + " in the catalog.")
		}
		if in.Overrides == nil {
			in.Overrides = map[string]json.RawMessage{}
		}
		return scan(c.DB.QueryRow(c.Ctx, `INSERT INTO tenant_services(tenant_id, service_id, enabled, overrides) VALUES($1::uuid, $2, $3, $4)
		  ON CONFLICT (tenant_id, service_id) DO UPDATE SET enabled=$3, overrides=$4, updated_at=now() RETURNING `+cols,
			in.CustomerID, in.ID, in.Enabled, in.Overrides))
	},
}
