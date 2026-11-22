package service

import "shippin.cloud/console/internal/mcpl"

// Delete is shippin.tenancy.customer.service.delete: DELETE /v1/operator/tenancy/customers/{customerId}/services/{serviceId}.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Drop a customer's own settings for a service, so its plan decides again.", Access: mcpl.Staff, Perm: "customers.services.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM tenant_services WHERE tenant_id::text=$1 AND service_id=$2`, in.CustomerID, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("This customer has no settings of its own for " + in.ID + ".")
		}
		return mcpl.None{}, err
	},
}
