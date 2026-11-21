package plan

import (
	"strconv"

	"shippin.cloud/console/internal/mcpl"
)

// Delete is shippin.tenancy.plan.delete: DELETE /v1/operator/tenancy/plans/{planId}. The plan's
// services go with it.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove a plan that no customer is on.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		// tenants.plan has no foreign key, so the customers on the plan are counted here.
		var n int
		if err := c.DB.QueryRow(c.Ctx, `SELECT count(*) FROM tenants WHERE plan=$1`, in.ID).Scan(&n); err != nil {
			return mcpl.None{}, err
		}
		if n > 0 {
			return mcpl.None{}, mcpl.Conflict(strconv.Itoa(n) + " customer(s) are still on this plan. Move them first.")
		}
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM plans WHERE id=$1`, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("No plan " + in.ID + ".")
		}
		return mcpl.None{}, err
	},
}
