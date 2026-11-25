package devspace

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// Listed is an organization's development spaces with what the page around them needs: the plan's
// limits for one space, and whether the caller manages the organization's spaces.
type Listed struct {
	Items []View `json:"items"`
	Plan  Plan   `json:"plan"`
	// CanManage: the caller may make and change spaces across the organization.
	CanManage    bool   `json:"can_manage"`
	TenantStatus string `json:"tenant_status"`
}

// Plan is the organization's plan and what one space on it may use.
type Plan struct {
	Name   string            `json:"name"`
	Limits ports.GuestLimits `json:"limits"`
}

// List is shippin.runtime.devspace.list: GET /v1/tenants/{tenantId}/runtime/devspaces. A person
// holding a support grant for one space is answered with that space only.
var List = mcpl.Def[struct{}, Listed]{
	Spec: mcpl.Spec{Summary: "List the organization's development spaces with their live state.", Access: mcpl.Member, Scoped: true},
	Run: func(c *mcpl.Call, _ struct{}) (Listed, error) {
		r, org, err := here(c)
		if err != nil {
			return Listed{}, err
		}
		items, err := r.Live(c.Ctx, *org, c.Sees)
		if err != nil {
			return Listed{}, err
		}
		for i := range items {
			items[i].Actions = offers(items[i], c.MayManage(items[i].ID))
		}
		return Listed{Items: items, Plan: Plan{Name: org.Plan, Limits: r.Guests.Limits(c.Ctx, org.Plan)},
			CanManage: c.Can("manage"), TenantStatus: org.Status}, nil
	},
}

// here is the reader and the organization a member action runs in.
func here(c *mcpl.Call) (Reader, *Org, error) {
	r, err := reader(c)
	if err != nil {
		return r, nil, err
	}
	orgs, err := mcpl.Need[Orgs](c)
	if err != nil {
		return r, nil, err
	}
	org, err := orgs.Org(c.Ctx, c.Tenant)
	if err != nil {
		return r, nil, err
	}
	if org == nil {
		return r, nil, mcpl.NotFound("Organization not found.")
	}
	return r, org, nil
}
