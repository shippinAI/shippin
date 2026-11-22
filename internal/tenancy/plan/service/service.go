// Package service is a plan's services: the cells of the plan x service matrix, which say whether a
// plan includes a catalog service, the lock mode a customer's own setting meets and the plan's limits.
// Staff with catalog.plans.write change them; they are read with their plan (tenancy/plan list and get),
// so the whole matrix is one read.
package service

import (
	"encoding/json"

	"shippin.cloud/console/internal/catalog"
)

// Parent names the plan from the route.
type Parent struct {
	PlanID string `path:"planId" json:"-"`
}

// ByID names one service of a plan from the route, by the catalog service's id.
type ByID struct {
	Parent
	ID string `path:"serviceId" json:"-"`
}

// Cell is what update takes. Enabled defaults to true, the lock to bounded and the limits to {}.
type Cell struct {
	ByID
	Enabled *bool        `json:"enabled,omitempty"`
	Lock    catalog.Lock `json:"lock,omitempty"`
	// Limits is a JSON object, such as {"seats": 3}.
	Limits json.RawMessage `json:"limits,omitempty"`
}
