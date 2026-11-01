package api

import (
	"context"
	"log"
	"slices"
	"time"

	"shippin.cloud/console/internal/db"
)

// staffView is what a caller may do as staff: their role keys and the union of those roles'
// permissions. Perms is empty without MFA (MFARequired is then true) or when anything failed.
type staffView struct {
	Roles       []string        `json:"roles"`
	Perms       map[string]bool `json:"-"`
	MFA         bool            `json:"mfa"`
	MFARequired bool            `json:"mfa_required"`
	Err         bool            `json:"error,omitempty"`
}

func (v *staffView) list() []string {
	out := []string{}
	for k := range v.Perms {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// permsFor unions the catalog permissions of the given role keys; unknown keys and
// permissions outside the catalog grant nothing.
func permsFor(roles []db.StaffRole, keys []string) map[string]bool {
	m := map[string]bool{}
	for _, r := range roles {
		if !slices.Contains(keys, r.Key) {
			continue
		}
		for _, p := range r.Permissions {
			if knownPerm(p) {
				m[p] = true
			}
		}
	}
	return m
}

// staffOf resolves (once per request, cached per user) the caller's staff view.
func (a *API) staffOf(ctx context.Context, c *caller) *staffView {
	if c.staff != nil {
		return c.staff
	}
	s := &a.staffCache
	s.mu.Lock()
	if e, ok := s.users[c.id.Sub]; ok && time.Since(e.at) < a.staffTTL() {
		s.mu.Unlock()
		c.staff = e.v
		return e.v
	}
	s.mu.Unlock()
	v := a.resolveStaff(ctx, c.id.Sub)
	if !v.Err {
		s.mu.Lock()
		if s.users == nil {
			s.users = map[string]staffEntry{}
		}
		s.users[c.id.Sub] = staffEntry{v, time.Now()}
		s.mu.Unlock()
	}
	c.staff = v
	return v
}

func (a *API) resolveStaff(ctx context.Context, sub string) *staffView {
	v := &staffView{Perms: map[string]bool{}, Roles: []string{}}
	g, err := a.idp().Grant(ctx, sub)
	if err != nil {
		if err != errNoTeamToken {
			log.Printf("staff: grants of %s: %v", sub, err)
		}
		v.Err = true
		return v
	}
	v.Roles = append(v.Roles, g.Roles...)
	if len(v.Roles) == 0 {
		return v
	}
	if v.MFA, err = a.idp().MFA(ctx, sub); err != nil {
		log.Printf("staff: MFA state of %s: %v", sub, err)
		v.Err = true
		return v
	}
	if !v.MFA {
		v.MFARequired = true // invariant: no staff permission without a second factor
		return v
	}
	roles, err := a.staffRoles(ctx)
	if err != nil {
		log.Printf("staff: roles table: %v", err)
		v.Err = true
		return v
	}
	v.Perms = permsFor(roles, v.Roles)
	// Direct grants (grantable permissions only) add to the roles; a staff role and MFA stay
	// required, and a failed read fails closed like the roles table.
	gs, err := a.grants().PermGrantsOf(ctx, sub)
	if err != nil {
		log.Printf("staff: direct grants of %s: %v", sub, err)
		v.Err, v.Perms = true, map[string]bool{}
		return v
	}
	for _, g := range gs {
		if grantablePerm(g.Permission) {
			v.Perms[g.Permission] = true
		}
	}
	return v
}
