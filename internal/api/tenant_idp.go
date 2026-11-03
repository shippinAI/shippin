package api

import (
	"context"
	"slices"

	"shippin.cloud/console/internal/auth"
)

// tenantIdP is the Nomen side of a tenant's Team page: org members and org-role writes in the
// tenant's own org, with the console's service token. Tests substitute a fake.
type tenantIdP interface {
	Members(ctx context.Context, org string) ([]auth.OrgMember, error)
	UserByEmail(ctx context.Context, email string) (string, error)
	Invite(ctx context.Context, org, email, given, family string) (string, error)
	SetMember(ctx context.Context, org, user string, roles []string, add bool) error
	RemoveMember(ctx context.Context, org, user string) error
}

type nomenOrgs struct{ a *API }

func (n nomenOrgs) tok() string { return n.a.cfg.IDPServiceToken }
func (n nomenOrgs) Members(ctx context.Context, org string) ([]auth.OrgMember, error) {
	return n.a.flow.OrgMembers(ctx, n.tok(), org, n.a.cfg.Operator.MemberLimit)
}
func (n nomenOrgs) UserByEmail(ctx context.Context, email string) (string, error) {
	return n.a.flow.UserByEmail(ctx, n.tok(), email)
}
func (n nomenOrgs) Invite(ctx context.Context, org, email, given, family string) (string, error) {
	return n.a.flow.InviteHuman(ctx, n.tok(), org, email, given, family)
}
func (n nomenOrgs) SetMember(ctx context.Context, org, user string, roles []string, add bool) error {
	return n.a.flow.SetOrgMember(ctx, n.tok(), org, user, roles, add)
}
func (n nomenOrgs) RemoveMember(ctx context.Context, org, user string) error {
	return n.a.flow.RemoveOrgMember(ctx, n.tok(), org, user)
}

func (a *API) orgIdP() tenantIdP {
	if a.tenantIdP != nil {
		return a.tenantIdP
	}
	return nomenOrgs{a}
}

// assignableRoles are the IdP org roles the Team page may grant: exactly roles.org's "from" list.
func (a *API) assignableRoles() []string {
	out := []string{}
	for _, m := range a.cfg.Roles.Org {
		if !slices.Contains(out, m.From) {
			out = append(out, m.From)
		}
	}
	return out
}

// ownerRole is the IdP org role that maps to the tenant owner role (roles.owner).
func (a *API) ownerRole() string {
	for _, m := range a.cfg.Roles.Org {
		if m.To == a.cfg.Roles.Owner {
			return m.From
		}
	}
	return ""
}

// roleChangeProblem says why a change of one member's org roles is refused, or "" if allowed:
// only known roles; only owners grant or take the owner role; the last owner keeps it.
func roleChangeProblem(allowed []string, owner string, callerOwner bool, before, after []string, owners int) string {
	for _, r := range after {
		if !slices.Contains(allowed, r) {
			return "Unknown role " + r + "."
		}
	}
	had, has := slices.Contains(before, owner), slices.Contains(after, owner)
	if had != has && !callerOwner {
		return "Only an owner can grant or remove the owner role."
	}
	if had && !has && owners <= 1 {
		return "This is the last owner; make someone else an owner first."
	}
	return ""
}
