package runner_size

import (
	"strconv"

	"shippin.cloud/console/internal/mcpl"
)

// Delete is shippin.services.runner_size.delete: DELETE /v1/operator/services/runner-sizes/{runnerSizeId}.
// A size a job is waiting for or running on stays until that job ends; turn it off instead.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove a runner size no waiting or running job needs.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		var busy int
		if err := c.DB.QueryRow(c.Ctx, `SELECT count(*) FROM shippin_runner_jobs WHERE size=$1 AND status IN ('queued','in_progress')`, in.ID).Scan(&busy); err != nil {
			return mcpl.None{}, err
		}
		if busy > 0 {
			return mcpl.None{}, mcpl.Conflict(strconv.Itoa(busy) + " job(s) are waiting for or running on this size. Turn it off instead, and remove it once they end.")
		}
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM shippin_runner_sizes WHERE id=$1`, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("No runner size " + in.ID + ".")
		}
		return mcpl.None{}, err
	},
}
