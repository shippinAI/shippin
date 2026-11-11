package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// tenantView is a tenant as the caller sees it: through org membership (Roles), or, for a
// platform admin with no membership, only through active access grants.
type tenantView struct {
	db.Tenant
	Roles  []string   `json:"roles"`
	grants []db.Grant // non-empty only for non-members
	// coverage is the organization's effective coverage tier; signIn is the sign-in rule it sets for
	// members. Staff reach a tenant through grants and their own rules.
	coverage, signIn string
}

// caller is the authenticated identity plus the tenants it may see.
type caller struct {
	id       *auth.Identity
	op       string
	tenants  []tenantView
	managers []string
	staff    *staffView // resolved lazily by staffOf
	founder  bool       // Nomen founder role from the token claims: passes every check
	fga      shadowCtx  // N2: OpenFGA shadow of every role decision (no-op when off)
	// installOwner is the local directory owner. There is no IdP to grant them the infra or founder
	// role, so infrastructure and approvals accept them by name. This is not founder: tenant access
	// grants still apply to them.
	installOwner bool
	// infraGranted is set by the infrastructure gate once this request holds its permission.
	infraGranted bool
	// note is why this change is being made, read by the same gate.
	note string
	// method is how this session was signed in.
	method string
}

func (a *API) caller(w http.ResponseWriter, r *http.Request) (*caller, bool) {
	var c *caller
	var err error
	if uid := auth.LocalUser(r.Context()); uid != "" && auth.FromContext(r.Context()) == "" {
		c, err = a.localCaller(r, uid)
	} else {
		c, err = a.callerFor(r, auth.FromContext(r.Context()))
	}
	if err == errIdP {
		auth.Problem(w, http.StatusUnauthorized, "Your session expired. Sign in again.")
		return nil, false
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return nil, false
	}
	return c, true
}

var errIdP = errors.New("idp")

// callerFor resolves a token into identity, operator role and visible tenants. Staff roles
// never reveal tenants by themselves: staff see a non-member tenant only while they hold an
// approved (or break-glass) grant for it.
func (a *API) callerFor(r *http.Request, tok string) (*caller, error) {
	id, err := a.flow.Whoami(r.Context(), tok)
	if err != nil {
		return nil, errIdP
	}
	return a.callerFromIdentity(r, id)
}

// callerFromIdentity resolves an identity (from the IdP, or a bridge session's snapshot) into
// operator role and visible tenants.
func (a *API) callerFromIdentity(r *http.Request, id *auth.Identity) (*caller, error) {
	noteCaller(r.Context(), id.Sub)
	c := &caller{id: id, managers: a.cfg.Roles.Managers, fga: shadowCtx{a.n2.fga, r.Context(), a.cfg.Roles}}
	c.founder = a.claimRole(id, a.cfg.Identity.FounderRole)
	if c.founder {
		c.op = a.cfg.Operator.Role
	}
	roles := map[string][]string{}
	for _, m := range id.Memberships {
		if m.IAM {
			if !c.founder {
				c.op = a.opRole(m.Roles)
			}
			continue
		}
		for _, ro := range m.Roles {
			roles[m.OrgID] = append(roles[m.OrgID], a.orgRole(ro))
		}
	}
	if id.OrgID != "" && roles[id.OrgID] == nil {
		roles[id.OrgID] = []string{a.cfg.Roles.Member}
	}
	orgs := []string{}
	for o := range roles {
		orgs = append(orgs, o)
	}
	ts, err := a.db.TenantsByOrgs(r.Context(), orgs)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(ts))
	for _, t := range ts {
		ids = append(ids, t.ID)
	}
	cover, err := a.tenantCoverage(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	c.method = auth.SignInMethod(r.Context())
	for _, t := range ts {
		c.tenants = append(c.tenants, tenantView{Tenant: t, Roles: roles[t.IdpOrgID], coverage: cover[t.ID].Tier, signIn: cover[t.ID].SignIn})
	}
	// Staff who may hold grants see the granted tenants (only those, only while active).
	if a.decideQuiet(r, c, "access.request") || a.decideQuiet(r, c, "access.breakglass") {
		if err := a.addGrantedTenants(r, c); err != nil {
			return nil, err
		}
	}
	c.installOwner = strings.HasPrefix(id.Sub, localSubPrefix) && c.op != "" && c.op == a.cfg.Operator.Role
	owner := a.cfg.Roles.Owner
	slices.SortStableFunc(c.tenants, func(x, y tenantView) int { return rank(x, owner) - rank(y, owner) })
	a.syncAccount(r.Context(), c)
	return c, nil
}

func (c *caller) tenant(w http.ResponseWriter, id string) (*tenantView, bool) {
	for i := range c.tenants {
		if c.tenants[i].ID == id {
			if t := &c.tenants[i]; t.grants == nil && !signInMeets(c.method, t.signIn) {
				auth.Problem(w, http.StatusForbidden, signInRefusal(t.signIn))
				return nil, false
			}
			return &c.tenants[i], true
		}
	}
	auth.Problem(w, http.StatusNotFound, "Tenant not found.")
	return nil, false
}

func rank(t tenantView, owner string) int {
	if slices.Contains(t.Roles, owner) {
		return 0
	}
	return 1
}
