// Package approval is who may approve what. Hosts, subjects, and roles come from
// configuration (and env values copied into that configuration at startup), not from
// names compiled into the console.
package approval

import "strings"

// Subject is the signed-in caller the policy evaluates.
type Subject struct {
	Sub           string
	Roles         []string
	Email         string
	EmailVerified bool
}

// Rule is one row of the approval policy.
type Rule struct {
	ID             string   `json:"id"`
	Actions        []string `json:"actions"`
	Hosts          []string `json:"hosts"` // empty: every host
	ApproverRoles  []string `json:"approver_roles"`
	ApproverSubs   []string `json:"approver_subs"`
	ApproverEmails []string `json:"approver_emails"`
	NeedsApproval  bool     `json:"needs_approval"`
}

// BoardConfig selects how approval requests reach the factory board.
type BoardConfig struct {
	// Driver is "hub" (this console's agent checkout board), "http" (a remote board), or "off".
	Driver   string `json:"driver"`
	BaseURL  string `json:"base_url"`
	OrgID    string `json:"org_id"`
	TokenEnv string `json:"token_env"`
}

// Policy is the configured approval policy.
type Policy struct {
	Enabled bool        `json:"enabled"`
	Board   BoardConfig `json:"board"`
	Rules   []Rule      `json:"rules"`
}

// HasAction reports whether any rule names action.
func (p Policy) HasAction(action string) bool {
	for _, r := range p.Rules {
		if has(r.Actions, action) {
			return true
		}
	}
	return false
}

// RequiresReview reports whether a matching rule requires a human review.
// The board driver's configuration must never turn a required review into direct apply.
func (p Policy) RequiresReview(action, host string) bool {
	r, ok := p.match(action, host)
	return ok && r.NeedsApproval
}

// Allows reports whether sub may perform action on host.
func (p Policy) Allows(sub Subject, action, host string) bool {
	if !p.Enabled {
		return false
	}
	r, ok := p.match(action, host)
	if !ok {
		return false
	}
	if sub.Sub != "" && has(r.ApproverSubs, sub.Sub) {
		return true
	}
	if sub.EmailVerified && sub.Email != "" && hasFold(r.ApproverEmails, sub.Email) {
		return true
	}
	for _, role := range sub.Roles {
		if role != "" && has(r.ApproverRoles, role) {
			return true
		}
	}
	return false
}

func (p Policy) match(action, host string) (Rule, bool) {
	for _, r := range p.Rules {
		if !has(r.Actions, action) {
			continue
		}
		if host != "" && len(r.Hosts) > 0 && !has(r.Hosts, host) && !has(r.Hosts, "*") {
			continue
		}
		return r, true
	}
	return Rule{}, false
}

func has(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func hasFold(ss []string, v string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}
