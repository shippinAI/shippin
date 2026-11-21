package api

import (
	"context"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/ext"
)

// callerPolicy is the registered module's caller policy (internal/ext), else the panel's own.
func (a *API) callerPolicy() ext.CallerPolicy {
	if p := ext.Caller(); p != nil {
		return p
	}
	return currentCaller{a}
}

// runtime is what the panel hands a module's policies and jobs: its database and its action services.
func (a *API) runtime() ext.Runtime {
	rt := ext.Runtime{Services: a.actionServices()}
	if a.db != nil {
		rt.Pool = a.db.Pool
	}
	return rt
}

// orgRules is each organization's label and sign-in rule, from the caller policy.
func (a *API) orgRules(ctx context.Context, ids []string) (map[string]ext.OrgRule, error) {
	if len(ids) == 0 {
		return map[string]ext.OrgRule{}, nil
	}
	rules, err := a.callerPolicy().OrgRules(ctx, a.runtime(), ids)
	if rules == nil {
		rules = map[string]ext.OrgRule{}
	}
	return rules, err
}

// currentCaller is the caller policy this repository still carries: coverage tiers' sign-in rules, and
// support and break-glass grants for staff. Both move to the enterprise module (roadmap S3), and then
// the panel's own answer is no rule and no access beyond membership.
type currentCaller struct{ a *API }

func (p currentCaller) OrgRules(ctx context.Context, _ ext.Runtime, orgs []string) (map[string]ext.OrgRule, error) {
	cover, err := p.a.tenantCoverage(ctx, orgs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ext.OrgRule, len(cover))
	for id, c := range cover {
		out[id] = ext.OrgRule{Label: c.Tier, SignIn: c.SignIn}
	}
	return out, nil
}

// Access is the active grants of staff who may hold them.
func (p currentCaller) Access(ctx context.Context, _ ext.Runtime, sub string, may func(string) bool) (map[string][]ext.Access, error) {
	if p.a.db == nil || !(may("access.request") || may("access.breakglass")) {
		return nil, nil
	}
	gs, err := p.a.db.ActiveGrants(ctx, sub)
	if err != nil {
		return nil, err
	}
	out := map[string][]ext.Access{}
	for _, g := range gs {
		out[g.TenantID] = append(out[g.TenantID], accessOf(g))
	}
	return out, nil
}

func accessOf(g db.Grant) ext.Access {
	x := ext.Access{ID: g.ID, OrgID: g.TenantID, WorkspaceID: g.WorkspaceID, WorkspaceName: g.WorkspaceName,
		Kind: g.Kind, BreakGlass: g.Kind == "breakglass"}
	if g.ExpiresAt != nil {
		x.ExpiresAt = *g.ExpiresAt
	}
	return x
}

// grantFromAccess is a policy's access as the grant record the panel's access checks and audit read today. An
// access past its expiry reads as expired. When access moves to the enterprise module (S3), the panel
// keeps ext.Access instead.
func grantFromAccess(org string, x ext.Access) db.Grant {
	kind := x.Kind
	if x.BreakGlass {
		kind = "breakglass"
	} else if kind == "breakglass" || kind == "" {
		kind = "request"
	}
	g := db.Grant{ID: x.ID, TenantID: org, WorkspaceID: x.WorkspaceID, WorkspaceName: x.WorkspaceName, Kind: kind, Status: "approved"}
	if !x.ExpiresAt.IsZero() {
		exp := x.ExpiresAt
		g.ExpiresAt = &exp
	}
	return g
}
