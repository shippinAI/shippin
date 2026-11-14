package customer

import (
	"github.com/google/uuid"

	"shippin.cloud/console/internal/mcpl"
)

// CreateInput is a new customer's fields; its id, status and sign-in link are the server's.
type CreateInput struct {
	Fields
}

// Create is shippin.tenancy.customer.create: POST /v1/operator/tenancy/customers.
var Create = mcpl.Def[CreateInput, Customer]{
	Spec: mcpl.Spec{Summary: "Add a draft customer on a plan, before its organization is provisioned.", Access: mcpl.Staff, Perm: "customers.write"},
	Run: func(c *mcpl.Call, in CreateInput) (Customer, error) {
		if err := in.check(c); err != nil {
			return Customer{}, err
		}
		if (len(in.Contacts) > 0 || len(in.Endpoints) > 0) && !c.Can(permContacts) || in.Notes != "" && !c.Can(permNotes) {
			return Customer{}, mcpl.Forbidden("Contacts and endpoints need customers.contacts.read; notes need customers.notes.read.")
		}
		id, err := uuid.NewV7()
		if err != nil {
			return Customer{}, err
		}
		_, err = c.DB.Exec(c.Ctx, `INSERT INTO tenants(id, idp_org_id, name, plan, status, contacts, endpoints, notes, coverage)
		  VALUES($1, $2, $3, $4, 'draft', $5, $6, $7, $8)`,
			id.String(), unlinkedOrg+id.String(), in.Name, in.PlanID, in.Contacts, in.Endpoints, in.Notes, in.Coverage)
		if err != nil {
			return Customer{}, err
		}
		return one(c, id.String())
	},
}
