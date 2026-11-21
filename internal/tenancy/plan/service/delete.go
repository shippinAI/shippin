package service

import "shippin.cloud/console/internal/mcpl"

// Delete is shippin.tenancy.plan.service.delete: DELETE /v1/operator/tenancy/plans/{planId}/services/{serviceId}.
// Without its cell the service is not included in the plan, unless the catalog makes it required.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Stop a plan from including a catalog service.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM plan_services WHERE plan_id=$1 AND service_id=$2`, in.PlanID, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("Plan " + in.PlanID + " doesn't include the service " + in.ID + ".")
		}
		return mcpl.None{}, err
	},
}
