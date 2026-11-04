// Package approval evaluates access to Shippin's local agent task board. It is not
// AgentHub review policy and cannot authorize platform provider changes.
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
}

// BoardConfig controls the optional local agent task board feature.
type BoardConfig struct {
	Driver string `json:"driver"`
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
		if ruleMatches(r, action, host) {
			return r, true
		}
	}
	return Rule{}, false
}

func ruleMatches(r Rule, action, host string) bool {
	if !has(r.Actions, action) {
		return false
	}
	return host == "" || len(r.Hosts) == 0 || has(r.Hosts, host) || has(r.Hosts, "*")
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
