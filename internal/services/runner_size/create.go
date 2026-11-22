package runner_size

import "shippin.cloud/console/internal/mcpl"

// CreateInput is a new size: its id, which jobs name and which never changes, and its fields.
type CreateInput struct {
	ID string `json:"id"`
	Fields
}

// Create is shippin.services.runner_size.create: POST /v1/operator/services/runner-sizes.
var Create = mcpl.Def[CreateInput, Size]{
	Spec: mcpl.Spec{Summary: "Offer a runner size: its label, cores, memory, disk and image.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in CreateInput) (Size, error) {
		if err := in.check(in.ID); err != nil {
			return Size{}, err
		}
		_, err := c.DB.Exec(c.Ctx, `INSERT INTO shippin_runner_sizes(id,label,cores,memory_mib,disk_gib,image,active) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			in.ID, in.Label, in.Cores, in.MemoryMiB, in.DiskGiB, in.Image, active(in.Active))
		if taken(err) {
			return Size{}, mcpl.Conflict("There is already a runner size " + in.ID + ".")
		}
		if err != nil {
			return Size{}, err
		}
		return one(c, in.ID)
	},
}
