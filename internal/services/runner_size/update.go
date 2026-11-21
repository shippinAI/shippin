package runner_size

import "shippin.cloud/console/internal/mcpl"

// UpdateInput is a size's new fields. Its id stays: jobs keep asking for it by that name.
type UpdateInput struct {
	ByID
	Fields
}

// Update is shippin.services.runner_size.update: PUT /v1/operator/services/runner-sizes/{runnerSizeId}.
// Turning a size off stops new jobs from getting it; running ones finish.
var Update = mcpl.Def[UpdateInput, Size]{
	Spec: mcpl.Spec{Summary: "Change a runner size, or stop offering it.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (Size, error) {
		if err := in.check(in.ID); err != nil {
			return Size{}, err
		}
		tag, err := c.DB.Exec(c.Ctx, `UPDATE shippin_runner_sizes SET label=$2, cores=$3, memory_mib=$4, disk_gib=$5, image=$6, active=$7, updated_at=now()
		  WHERE id=$1`, in.ID, in.Label, in.Cores, in.MemoryMiB, in.DiskGiB, in.Image, active(in.Active))
		if err != nil {
			return Size{}, err
		}
		if tag.RowsAffected() == 0 {
			return Size{}, mcpl.NotFound("No runner size " + in.ID + ".")
		}
		return one(c, in.ID)
	},
}
