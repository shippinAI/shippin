package config

// Operator configures the provider master panel (/admin): who may open it and what it checks.
type Operator struct {
	// Role is the operator role required (mapped from Nomen IAM roles), the platform admin role.
	Role string `json:"role"`
	// PlatformOrgID is the provider org whose users/apps are listed.
	PlatformOrgID string `json:"platform_org_id"`
	// CertHosts are TLS names checked for expiry via the edge (Dial).
	CertHosts []string `json:"cert_hosts"`
	// AdminHosts are platform tools behind the edge's forward_auth (e.g. the mail admin);
	// only the platform admin role may open them.
	AdminHosts []string `json:"admin_hosts"`
	// Host is the operator app's own origin (ops.<domain>, CONSOLE-IA); "" = the panel only lives
	// under console_host/admin. ClientID is its own IdP app (audience); SHIPPIN_OPS_OIDC_CLIENT_ID
	// overrides it. OriginOnly refuses the operator API on every other host.
	Host       string `json:"host"`
	ClientID   string `json:"client_id"`
	OriginOnly bool   `json:"origin_only"`
	// MemberLimit caps the IdP org members listed on a tenant detail page.
	MemberLimit int `json:"member_limit"`
}
