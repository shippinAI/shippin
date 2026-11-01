package config

// Identity is the one shared claim mapping: Nomen (Zitadel) project roles delivered in the
// token/userinfo claims decide who is founder, admin or user, and which tenant a user is in.
// No service keeps a local admin list; every name here is data (config/shippin.example.json).
type Identity struct {
	// RoleClaims are the claim names holding project roles ({role: {orgId: domain}}).
	// "{project}" is replaced by staff.project_id.
	RoleClaims []string `json:"role_claims"`
	// FounderRole passes every Shippin access check (console, edge hosts, workspaces, chat
	// admin, launcher). AdminRole maps to the operator role in operator.role's place only
	// for Admin pages; UserRole is a plain signed-in user.
	FounderRole string `json:"founder_role"`
	AdminRole   string `json:"admin_role"`
	UserRole    string `json:"user_role"`
	// TenantClaim is the claim carrying the caller's tenant (Nomen org id by default).
	TenantClaim string `json:"tenant_claim"`
}

// IdentityDefaults fills unset fields.
func (c *Config) IdentityDefaults() {
	i := &c.Identity
	if len(i.RoleClaims) == 0 {
		i.RoleClaims = []string{"urn:nomen:iam:org:project:{project}:roles", "urn:nomen:iam:org:project:roles"}
	}
	if i.FounderRole == "" {
		i.FounderRole = "founder"
	}
	if i.AdminRole == "" {
		i.AdminRole = "admin"
	}
	if i.UserRole == "" {
		i.UserRole = "user"
	}
	if i.TenantClaim == "" {
		i.TenantClaim = "urn:nomen:iam:user:resourceowner:id"
	}
}
