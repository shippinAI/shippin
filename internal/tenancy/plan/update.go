package plan

import "shippin.cloud/console/internal/mcpl"

// UpdateInput is a plan's new fields. A price left out, or sent by a caller without
// catalog.prices.read, keeps the stored one.
type UpdateInput struct {
	ByID
	Fields
}

// Update is shippin.tenancy.plan.update: PUT /v1/operator/tenancy/plans/{planId}.
var Update = mcpl.Def[UpdateInput, Plan]{
	Spec: mcpl.Spec{Summary: "Change a plan's label, description, price, rank, visibility or coverage tier.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (Plan, error) {
		if err := in.check(c, in.ID); err != nil {
			return Plan{}, err
		}
		if !c.Can(permPrices) {
			in.PriceCents = nil
		}
		tag, err := c.DB.Exec(c.Ctx, `UPDATE plans SET label=$2, description=$3, rank=$4, price_cents=coalesce($5, price_cents), public=$6,
		  active=$7, coverage=$8, updated_at=now() WHERE id=$1`,
			in.ID, in.Label, in.Description, in.Rank, in.PriceCents, boolOr(in.Public), boolOr(in.Active), in.Coverage)
		if err != nil {
			return Plan{}, err
		}
		if tag.RowsAffected() == 0 {
			return Plan{}, mcpl.NotFound("No plan " + in.ID + ".")
		}
		return one(c, in.ID)
	},
}
