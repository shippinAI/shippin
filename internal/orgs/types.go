// Package orgs is the CP-P0b org split: one Operations org (Shippin.cloud's own control plane) and
// customer orgs. Staff roles exist only on Operations and never grant tenant data; support access
// to a customer org is a separate, time-limited, audited grant.
package orgs

import (
	"errors"
	"regexp"
	"slices"
	"time"
)

const (
	TypeOperations = "operations"
	TypeCustomer   = "customer"
)

// Staff roles (Operations org only). Tenant roles stay in the existing org_owner/org_admin model.
const (
	RoleAdmin   = "ops_admin"
	RoleOncall  = "ops_oncall"
	RoleSupport = "ops_support"
	RoleAuditor = "ops_auditor"
)

var StaffRoles = []string{RoleAdmin, RoleOncall, RoleSupport, RoleAuditor}

type Org struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	NomenOrgID string `json:"nomen_org_id,omitempty"`
}

type StaffRole struct {
	UserSub   string     `json:"user_sub"`
	Role      string     `json:"role"`
	GrantedBy string     `json:"granted_by"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type SupportGrant struct {
	ID        string     `json:"id"`
	StaffSub  string     `json:"staff_sub"`
	OrgID     string     `json:"org_id"`
	ProjectID string     `json:"project_id,omitempty"`
	Reason    string     `json:"reason"`
	GrantedBy string     `json:"granted_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Active reports whether the grant is usable at now.
func (g SupportGrant) Active(now time.Time) bool { return g.RevokedAt == nil && now.Before(g.ExpiresAt) }

var (
	ErrNotFound   = errors.New("not found")
	ErrBadInput   = errors.New("bad input")
	ErrTooLong    = errors.New("grant longer than the configured maximum")
	reSlug        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	reOrgID       = regexp.MustCompile(`^org_[0-9A-HJKMNP-TV-Z]{26}$`)
	staffReadable = []string{RoleAdmin, RoleOncall, RoleSupport, RoleAuditor}
)

// ValidSlug / ValidOrgID check shapes before any query.
func ValidSlug(s string) bool  { return reSlug.MatchString(s) }
func ValidOrgID(s string) bool { return reOrgID.MatchString(s) }

// CanReadPlatform: any staff role may view servers/tenants; it needs no devspace.
func CanReadPlatform(roles []string) bool {
	return slices.ContainsFunc(roles, func(r string) bool { return slices.Contains(staffReadable, r) })
}

// CanAdminPlatform: only ops_admin changes platform objects and issues support grants.
func CanAdminPlatform(roles []string) bool { return slices.Contains(roles, RoleAdmin) }
