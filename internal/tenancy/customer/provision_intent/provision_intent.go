// Package provision_intent is a customer's request to be provisioned: a snapshot of its merged
// configuration (plan, services and settings) recorded as the state it should reach. Recording one starts
// nothing; it stays pending until an operator applies it through change control. Staff with
// provision.intent.create record them; staff with customers.read read them. Its actions are the other
// files in this folder.
package provision_intent

import (
	"time"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/customer"
)

// Status is where a request stands; kept in step with the CHECK constraint in migration 010.
type Status string

func (Status) Values() []string {
	return []string{"pending", "ticketed", "applied", "failed", "cancelled"}
}

// Intent is one provisioning request, with the services it would turn on.
type Intent struct {
	ID              string    `json:"id"`
	CustomerID      string    `json:"tenant_id"`
	Status          Status    `json:"status"`
	TicketRef       string    `json:"ticket_ref"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	EnabledServices []string  `json:"enabled_services"`
}

// Parent names the customer, from the route.
type Parent struct {
	CustomerID string `path:"customerId" json:"-"`
}

const cols = `id::text, tenant_id::text, status, ticket_ref, created_by, created_at, desired->'enabled_services'`

func scan(row pgx.Row) (Intent, error) {
	var i Intent
	err := row.Scan(&i.ID, &i.CustomerID, &i.Status, &i.TicketRef, &i.CreatedBy, &i.CreatedAt, &i.EnabledServices)
	if i.EnabledServices == nil {
		i.EnabledServices = []string{}
	}
	return i, err
}

// must answers 404 unless the route names a customer.
func (p Parent) must(c *mcpl.Call) error {
	ok, err := customer.Exists(c, p.CustomerID)
	if err == nil && !ok {
		err = customer.NotFound()
	}
	return err
}
