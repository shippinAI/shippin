package api

import (
	"net/http"
	"regexp"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/orgs"
)

func (a *API) orgStore() *orgs.Store { return &orgs.Store{Pool: a.db.Pool} }

type opsHandler func(http.ResponseWriter, *http.Request, *caller, []string)

// opsGate admits callers holding an Operations staff role. It needs no tenant and no devspace;
// tenant roles (org_owner, org_admin, …) never pass. admin=true additionally requires ops_admin.
func (a *API) opsGate(admin bool, h opsHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.Orgs.Enabled {
			auth.Problem(w, http.StatusNotFound, "Operations isn't enabled.")
			return
		}
		c, ok := a.resolve(w, r)
		if !ok {
			return
		}
		roles, err := a.opsRoles(r, c)
		if err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "Staff roles are unavailable right now.")
			return
		}
		if !orgs.CanReadPlatform(roles) || (admin && !orgs.CanAdminPlatform(roles)) {
			auth.Problem(w, http.StatusForbidden, "This needs an Operations staff role. Tenant roles don't grant it.")
			return
		}
		h(w, r, c, roles)
	}
}

// opsRoles reads the caller's active ops roles. The configured bootstrap subject becomes ops_admin
// the first time it is seen (and the Operations org is created), so setup needs no database access.
func (a *API) opsRoles(r *http.Request, c *caller) ([]string, error) {
	s := a.orgStore()
	roles, err := s.StaffRolesOf(r.Context(), c.id.Sub)
	if err != nil || len(roles) > 0 || a.cfg.Orgs.BootstrapAdminSub == "" || c.id.Sub != a.cfg.Orgs.BootstrapAdminSub {
		return roles, err
	}
	o := a.cfg.Orgs
	if _, err := s.EnsureOperations(r.Context(), o.OperationsSlug, o.OperationsName, o.OperationsNomenOrgID); err != nil {
		return nil, err
	}
	if err := s.GrantStaffRole(r.Context(), c.id.Sub, orgs.RoleAdmin, "config:bootstrap_admin_sub", nil); err != nil {
		return nil, err
	}
	return []string{orgs.RoleAdmin}, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9-]+`)

// orgSlug derives a slug from a tenant name (customer orgs are created on first sight).
func orgSlug(name, fallback string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	if s == "" {
		s = "org"
	}
	if f := nonSlug.ReplaceAllString(strings.ToLower(fallback), ""); len(f) >= 6 {
		s += "-" + f[len(f)-6:] // the random tail (UUIDv7 heads share the timestamp)
	}
	return nonSlug.ReplaceAllString(s, "-")
}
