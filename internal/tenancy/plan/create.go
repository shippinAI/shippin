package plan

import "shippin.cloud/console/internal/mcpl"

// CreateInput is a new plan: its id, which never changes, and its fields.
type CreateInput struct {
	ID string `json:"id"`
	Fields
}

// Create is shippin.tenancy.plan.create: POST /v1/operator/tenancy/plans.
var Create = mcpl.Def[CreateInput, Plan]{
	Spec: mcpl.Spec{Summary: "Create a plan with its label, price, rank and the coverage tier it includes.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in CreateInput) (Plan, error) {
		if err := in.check(c, in.ID); err != nil {
			return Plan{}, err
		}
		cents := 0
		if in.PriceCents != nil {
			cents = *in.PriceCents
		}
		if cents != 0 && !c.Can(permPrices) {
			return Plan{}, mcpl.Forbidden("Setting a price needs catalog.prices.read.")
		}
		_, err := c.DB.Exec(c.Ctx, `INSERT INTO plans(id,label,description,rank,price_cents,public,active,coverage) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			in.ID, in.Label, in.Description, in.Rank, cents, boolOr(in.Public), boolOr(in.Active), in.Coverage)
		if taken(err) {
			return Plan{}, mcpl.Conflict("There is already a plan " + in.ID + ". Pick another id.")
		}
		if err != nil {
			return Plan{}, err
		}
		return one(c, in.ID)
	},
}
