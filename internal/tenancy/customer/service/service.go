// Package service is a customer's own settings for one catalog service: switched on or off against what
// its plan gives, and settings that override the catalog's defaults. Staff with customers.services.write
// change them; the merge into the customer's effective configuration (catalog.EffectiveFor) reads them,
// within the plan's locks and bounds. Its actions are the other files in this folder.
package service

import (
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/customer"
)

// Override is a customer's own settings for one service. A null Enabled follows the plan.
type Override struct {
	CustomerID string                     `json:"tenant_id"`
	ServiceID  string                     `json:"service_id"`
	Enabled    *bool                      `json:"enabled"`
	Overrides  map[string]json.RawMessage `json:"overrides"`
}

// Parent names the customer, from the route.
type Parent struct {
	CustomerID string `path:"customerId" json:"-"`
}

// ByID names one service of the customer, by the catalog service's id.
type ByID struct {
	Parent
	ID string `path:"serviceId" json:"-"`
}

// must answers 404 unless the route names a customer.
func (p Parent) must(c *mcpl.Call) error {
	ok, err := customer.Exists(c, p.CustomerID)
	if err == nil && !ok {
		err = customer.NotFound()
	}
	return err
}

func scan(row pgx.Row) (Override, error) {
	var o Override
	err := row.Scan(&o.CustomerID, &o.ServiceID, &o.Enabled, &o.Overrides)
	if o.Overrides == nil {
		o.Overrides = map[string]json.RawMessage{}
	}
	return o, err
}

const cols = `tenant_id::text, service_id, enabled, overrides`
