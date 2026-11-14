package catalog_service

import (
	"strconv"

	"shippin.cloud/console/internal/mcpl"
)

// Delete is shippin.services.catalog_service.delete: DELETE /v1/operator/services/catalog-services/{catalogServiceId}.
// The plans that include the service and organizations' settings for it go with it (those tables
// cascade); a deployment record keeps it in the catalog.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove a service from the catalog; plans and organizations lose it, and it is refused while a deployment uses it.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		var n int
		if err := c.DB.QueryRow(c.Ctx, `SELECT count(*) FROM shippin_deployments WHERE service_id=$1`, in.ID).Scan(&n); err != nil {
			return mcpl.None{}, err
		}
		if n > 0 {
			return mcpl.None{}, mcpl.Conflict(strconv.Itoa(n) + " deployment(s) use this service, so it stays in the catalog. Make it inactive instead.")
		}
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM catalog_services WHERE id=$1`, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("No catalog service " + in.ID + ".")
		}
		return mcpl.None{}, err
	},
}
