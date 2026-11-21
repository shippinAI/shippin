// Package github_installation is an organization's connections to GitHub: each installation of the
// panel's GitHub App on a GitHub account or organization, bound to this organization when an owner
// or admin connected it. Connecting is the GitHub install flow (GET /v1/tenants/{tenantId}/github/install,
// a transport, since GitHub's own pages are in the middle); members read the connections here, without
// asking GitHub, and the repositories they reach are github_repository.
package github_installation

import "time"

// Installation is one GitHub connection.
type Installation struct {
	ID           string `json:"id"`
	AccountLogin string `json:"account_login"`
	// AccountType is GitHub's: User, Organization or Enterprise.
	AccountType string `json:"account_type"`
	// Suspended is true while the account has suspended the App; its repositories get no runners.
	Suspended bool      `json:"suspended"`
	CreatedAt time.Time `json:"created_at"`
}

// Connections are the organization's connections, and whether the caller may add one (owners and
// admins may).
type Connections struct {
	Items      []Installation `json:"items"`
	CanConnect bool           `json:"can_connect"`
}
