package customer

import "shippin.cloud/console/internal/mcpl"

// UpdateInput is a customer's new fields. Its status is not here: it changes through provisioning.
type UpdateInput struct {
	ByID
	Fields
}

// Update is shippin.tenancy.customer.update: PUT /v1/operator/tenancy/customers/{customerId}.
var Update = mcpl.Def[UpdateInput, Customer]{
	Spec: mcpl.Spec{Summary: "Change a customer's name, plan, coverage, contacts, endpoints or notes.", Access: mcpl.Staff, Perm: "customers.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (Customer, error) {
		if err := in.check(c); err != nil {
			return Customer{}, err
		}
		// A field the caller may not see is kept as stored (null keeps it), never blanked.
		var contacts, endpoints, notes any
		if c.Can(permContacts) {
			contacts, endpoints = in.Contacts, in.Endpoints
		}
		if c.Can(permNotes) {
			notes = in.Notes
		}
		tag, err := c.DB.Exec(c.Ctx, `UPDATE tenants SET name=$2, plan=$3, contacts=coalesce($4, contacts), endpoints=coalesce($5, endpoints),
		  notes=coalesce($6, notes), coverage=$7, updated_at=now() WHERE kind='customer' AND id::text=$1`,
			in.ID, in.Name, in.PlanID, contacts, endpoints, notes, in.Coverage)
		if err != nil {
			return Customer{}, err
		}
		if tag.RowsAffected() == 0 {
			return Customer{}, NotFound()
		}
		return one(c, in.ID)
	},
}
