package config

import "errors"

// RoleMap maps one IdP role key to a product role.
type RoleMap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Roles holds every role name the console uses; nothing is hardcoded in Go or the UI.
type Roles struct {
	// IAM maps Nomen instance roles to operator roles; the first match wins, so list the
	// strongest first. IAMDefault applies to any other instance member.
	IAM        []RoleMap `json:"iam"`
	IAMDefault string    `json:"iam_default"`
	// Org maps Nomen org roles to tenant roles. Unlisted roles become "org_<role>".
	Org []RoleMap `json:"org"`
	// Member is the tenant role of a user whose home org is the tenant but who has no grant.
	Member string `json:"member"`
	// Owner sorts first in the tenant switcher.
	Owner string `json:"owner"`
	// Managers may manage workspaces and approve, deny and revoke platform access requests.
	Managers []string `json:"managers"`
	// Infra is the single role that may call hypervisor routes. Empty means op_infra
	// (filled in Load). It is a role name, not an identity.
	Infra string `json:"infra"`
}

// Check fails closed: an empty role table would silently grant or deny everything.
func (r Roles) Check(platformAdmin string) error {
	switch {
	case len(r.IAM) == 0 || r.IAMDefault == "":
		return errors.New("roles.iam and roles.iam_default are required")
	case len(r.Org) == 0 || r.Member == "" || r.Owner == "" || len(r.Managers) == 0:
		return errors.New("roles.org, member, owner and managers are required")
	case platformAdmin == "":
		return errors.New("operator.role (the platform admin role) is required")
	}
	for _, m := range append(append([]RoleMap{}, r.IAM...), r.Org...) {
		if m.From == "" || m.To == "" {
			return errors.New("roles: every mapping needs from and to")
		}
	}
	for _, m := range r.IAM {
		if m.To == platformAdmin {
			return nil
		}
	}
	return errors.New("operator.role must be reachable from roles.iam")
}
