package api

import (
	"context"
	"slices"

	"shippin.cloud/console/internal/db"
)

// holder is an active operator-org user and the permissions their roles give.
type holder struct {
	user  staffUser
	roles []string
	perms map[string]bool
}

// staffHolders lists active operator-org users holding at least one staff role, with an
// optional override of one user's roles (to evaluate a change before making it).
func (a *API) staffHolders(ctx context.Context, roles []db.StaffRole, override map[string][]string) ([]holder, error) {
	users, err := a.idp().Users(ctx)
	if err != nil {
		return nil, err
	}
	grants, err := a.idp().AllGrants(ctx)
	if err != nil {
		return nil, err
	}
	out := []holder{}
	for _, u := range users {
		keys := grants[u.ID].Roles
		if o, ok := override[u.ID]; ok {
			keys = o
		}
		if u.State != "active" || len(keys) == 0 {
			continue
		}
		out = append(out, holder{user: u, roles: keys, perms: permsFor(roles, keys)})
	}
	return out, nil
}

// countWith counts holders with perm, skipping skip (a user id) and, when needMFA, anyone
// without a second factor.
func (a *API) countWith(ctx context.Context, hs []holder, perm, skip string, needMFA bool) int {
	n := 0
	for _, h := range hs {
		if h.user.ID == skip || !h.perms[perm] {
			continue
		}
		if needMFA {
			if ok, err := a.idp().MFA(ctx, h.user.ID); err != nil || !ok {
				continue
			}
		}
		n++
	}
	return n
}

// ownersAfter is the number of active Owners (holders of permOwner) after applying override
// and, optionally, a replaced role definition.
func (a *API) ownersAfter(ctx context.Context, override map[string][]string, replace *db.StaffRole, drop string) (int, error) {
	roles, err := a.store().StaffRoles(ctx)
	if err != nil {
		return 0, err
	}
	next := []db.StaffRole{}
	for _, r := range roles {
		switch {
		case r.Key == drop:
			continue
		case replace != nil && r.Key == replace.Key:
			next = append(next, *replace)
		default:
			next = append(next, r)
		}
	}
	hs, err := a.staffHolders(ctx, next, override)
	if err != nil {
		return 0, err
	}
	return a.countWith(ctx, hs, permOwner, "", false), nil
}

func hasAll(have map[string]bool, keys []string) bool {
	return !slices.ContainsFunc(keys, func(k string) bool { return !have[k] })
}
