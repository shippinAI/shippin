package devspace

import "shippin.cloud/console/internal/mcpl"

// ByID names one development space in the route.
type ByID struct {
	ID string `path:"devspaceId" json:"-"`
}

// Get is shippin.runtime.devspace.get: GET /v1/tenants/{tenantId}/runtime/devspaces/{devspaceId}.
var Get = mcpl.Def[ByID, View]{
	Spec: mcpl.Spec{Summary: "Read one development space: its live state, how to reach it and the steps of work in progress.", Access: mcpl.Member, Scoped: true},
	Run: func(c *mcpl.Call, in ByID) (View, error) {
		r, org, err := here(c)
		if err != nil {
			return View{}, err
		}
		items, err := r.Live(c.Ctx, *org, c.Sees)
		if err != nil {
			return View{}, err
		}
		for _, it := range items {
			if it.ID == in.ID {
				return it, nil
			}
		}
		return View{}, mcpl.NotFound("Development space not found.")
	},
}
