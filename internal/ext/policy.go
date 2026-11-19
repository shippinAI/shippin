package ext

import (
	"context"
	"time"
)

// Policies are the places where the panel needs a business rule (docs/editions.md). The panel asks the
// registered module's policy if one is set, and its own default otherwise. At most one module sets each.

// OrgRule is what an organization requires of its members, and the label shown with it.
type OrgRule struct {
	// Label names the rule's source for people, such as a coverage tier ("" for none).
	Label string
	// SignIn is the weakest sign-in its members may use: "", "password", "second_factor" or
	// "identity_provider". An unknown value is treated as the strictest.
	SignIn string
}

// Access is how a caller reaches an organization they are not a member of, such as a support grant
// the organization approved or a break-glass. Using it is audited.
type Access struct {
	ID    string
	OrgID string
	// WorkspaceID limits it to one workspace ("" for the whole organization); WorkspaceName is shown.
	WorkspaceID, WorkspaceName string
	// Kind is shown to people. BreakGlass marks its use in the audit log as break-glass.
	Kind       string
	BreakGlass bool
	ExpiresAt  time.Time
}

// Active says the access is in force at now.
func (a Access) Active(now time.Time) bool { return !a.ExpiresAt.IsZero() && now.Before(a.ExpiresAt) }

// Covers says the access reaches workspace ws ("" for the organization itself).
func (a Access) Covers(ws string) bool { return a.WorkspaceID == "" || a.WorkspaceID == ws }

// CallerPolicy decides what a signed-in caller may reach besides the organizations they belong to.
type CallerPolicy interface {
	// OrgRules gives the rule of each organization in orgs that has one.
	OrgRules(ctx context.Context, rt Runtime, orgs []string) (map[string]OrgRule, error)
	// Access lists the organizations sub reaches without membership, and how, by organization. may
	// says whether the caller holds a staff permission.
	Access(ctx context.Context, rt Runtime, sub string, may func(perm string) bool) (map[string][]Access, error)
}

// Caller is the registered module's caller policy, or nil when no module sets one.
func Caller() CallerPolicy {
	for _, m := range Modules() {
		if m.Caller != nil {
			return m.Caller
		}
	}
	return nil
}
