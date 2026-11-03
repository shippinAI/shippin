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

// Default is the shipped behavior: founder role (and any subject/e-mail supplied by env at
// startup) may approve Gold changes and hub decisions. No host, IP, or person is named here.
func Default() Policy {
	return Policy{
		Enabled: true,
		Board:   BoardConfig{Driver: "hub", TokenEnv: "SHIPPIN_HUB_TOKEN"},
		Rules: []Rule{
			{ID: "gold", Actions: []string{"gold.ticket", "gold.passkey", "gold.relay", "gold.bypass"},
				ApproverRoles: []string{"founder"}, NeedsApproval: true},
			{ID: "hub-decide", Actions: []string{"hub.decide"},
				ApproverRoles: []string{"founder"}, NeedsApproval: true},
			{ID: "hv-lifecycle", Actions: []string{"hv.lifecycle"},
				ApproverRoles: []string{"founder", "op_infra", "org_owner", "org_admin"}, NeedsApproval: true},
			{ID: "hv-attach", Actions: []string{"hv.attach"},
				ApproverRoles: []string{"founder", "op_infra"}},
		},
	}
}

// WithRuntime copies deployment-specific subjects into the matching rules. The values stay
// configuration (env), not source.
func (p Policy) WithRuntime(getenv func(string) string) Policy {
	if getenv == nil {
		return p
	}
	goldSub := strings.TrimSpace(getenv("GOLDSIGN_OWNER_SUB"))
	hubSub := strings.TrimSpace(getenv("HUB_OWNER_SUB"))
	hubEmail := strings.TrimSpace(getenv("HUB_OWNER_EMAIL"))
	for i := range p.Rules {
		if goldSub != "" && has(p.Rules[i].Actions, "gold.ticket") {
			p.Rules[i].ApproverSubs = add(p.Rules[i].ApproverSubs, goldSub)
		}
		if has(p.Rules[i].Actions, "hub.decide") {
			if hubSub != "" {
				p.Rules[i].ApproverSubs = add(p.Rules[i].ApproverSubs, hubSub)
			}
			if hubEmail != "" {
				p.Rules[i].ApproverEmails = add(p.Rules[i].ApproverEmails, hubEmail)
			}
		}
	}
	return p
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

// NeedsBoard reports whether a matching rule asks for a factory-board decision.
func (p Policy) NeedsBoard(action, host string) bool {
	r, ok := p.match(action, host)
	return ok && r.NeedsApproval && p.Board.Driver != "" && p.Board.Driver != "off"
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

func add(ss []string, v string) []string {
	if hasFold(ss, v) {
		return ss
	}
	return append(ss, v)
}
