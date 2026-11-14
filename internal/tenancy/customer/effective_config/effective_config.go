// Package effective_config is what a customer ends up with once every layer is merged: system defaults,
// its plan, its own settings and, when asked, one workspace's. Each value carries the layer it came from,
// its lock and the trail of what each layer tried. Nobody writes it; it is computed on read by
// catalog.EffectiveFor, the same merge a provisioning request snapshots. Staff with config.effective.read
// read it.
package effective_config

// Parent names the customer, from the route.
type Parent struct {
	CustomerID string `path:"customerId" json:"-"`
}

// Query is the customer and, optionally, one of its workspaces whose own layer is merged too.
type Query struct {
	Parent
	Workspace string `query:"workspace" json:"-"`
}
