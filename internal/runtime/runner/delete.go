package runner

import (
	"errors"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/mcpl"
)

// Delete is shippin.runtime.runner.delete: DELETE /v1/operator/runtime/runners/{runnerId}. It marks
// the machine for removal with the reason; RemoveMachine then deletes it from its host once this is
// committed, and the panel's sweep retries a removal that fails. The job it ran keeps its history.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove a stuck runner machine, with a reason.", Access: mcpl.Staff, Perm: "infra.guests.manage"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		var state State
		err := c.DB.QueryRow(c.Ctx, `UPDATE shippin_runner_machines SET state='removing', removal_reason=left($2, 300), updated_at=now()
		  WHERE id::text=$1 AND state <> 'removed' RETURNING state`, in.ID, "Removed by staff: "+c.Reason).Scan(&state)
		if !errors.Is(err, pgx.ErrNoRows) {
			return mcpl.None{}, err
		}
		var exists bool
		if err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS (SELECT 1 FROM shippin_runner_machines WHERE id::text=$1)`, in.ID).Scan(&exists); err != nil {
			return mcpl.None{}, err
		}
		if exists {
			return mcpl.None{}, mcpl.Conflict("Runner machine " + in.ID + " is already removed.")
		}
		return mcpl.None{}, mcpl.NotFound("No runner machine " + in.ID + ".")
	},
}
