package provision_intent

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/customer"
)

// CreateInput is an optional note kept with the request.
type CreateInput struct {
	Parent
	Note string `json:"note,omitempty"`
}

// Create is shippin.tenancy.customer.provision_intent.create: POST /v1/operator/tenancy/customers/{customerId}/provision-intents.
// It records desired state only: nothing is provisioned until an operator applies it.
var Create = mcpl.Def[CreateInput, Intent]{
	Spec: mcpl.Spec{Summary: "Record the customer's merged configuration as the state to provision; an operator applies it later.", Access: mcpl.Staff, Perm: "provision.intent.create"},
	Run: func(c *mcpl.Call, in CreateInput) (Intent, error) {
		eff, err := catalog.EffectiveFor(c.Ctx, c.DB, in.CustomerID, "")
		if errors.Is(err, catalog.ErrNotFound) {
			return Intent{}, customer.NotFound()
		}
		if err != nil {
			return Intent{}, err
		}
		desired := map[string]any{"plan_id": eff.PlanID, "enabled_services": eff.Enabled, "services": eff.Services,
			"settings": eff.Settings, "note": strings.TrimSpace(in.Note)}
		id, err := uuid.NewV7()
		if err != nil {
			return Intent{}, err
		}
		return scan(c.DB.QueryRow(c.Ctx, `INSERT INTO provision_intents(id, tenant_id, desired, created_by) VALUES($1, $2::uuid, $3, $4) RETURNING `+cols,
			id.String(), in.CustomerID, desired, c.Actor))
	},
}
