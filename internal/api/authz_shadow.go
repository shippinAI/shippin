package api

import (
	"context"
	"slices"

	"shippin.cloud/console/internal/authz"
	"shippin.cloud/console/internal/config"
)

// shadowCtx sends each role decision to OpenFGA as well (N2, mode "shadow"). The caller's
// org roles and access grants become contextual tuples, so the model, not a tuple copy,
// decides; the legacy answer is what the request gets.
type shadowCtx struct {
	s     *authz.Shadow
	ctx   context.Context
	roles config.Roles
}

func (x shadowCtx) compare(c *caller, t *tenantView, wsID, relation string, legacy bool) {
	if x.s == nil || c == nil || c.id == nil || t == nil {
		return
	}
	obj := "tenant:" + t.ID
	if wsID != "" {
		obj = "workspace:" + wsID
	}
	user := "user:" + c.id.Sub
	x.s.Compare(x.ctx, authz.Check{Tuple: authz.Tuple{User: user, Relation: relation, Object: obj},
		Context: x.tuples(user, t, wsID)}, legacy)
}

// tuples states what the console already knows about the caller and tenant t.
func (x shadowCtx) tuples(user string, t *tenantView, wsID string) []authz.Tuple {
	tenant := "tenant:" + t.ID
	var out []authz.Tuple
	if wsID != "" {
		out = append(out, authz.Tuple{User: tenant, Relation: "tenant", Object: "workspace:" + wsID})
	}
	if t.member() {
		for _, r := range t.Roles {
			out = append(out, authz.Tuple{User: user, Relation: x.relation(r), Object: tenant})
		}
		return out
	}
	if t.grantFor("") != nil {
		out = append(out, authz.Tuple{User: user, Relation: "grantee", Object: tenant})
	}
	if wsID != "" && t.grantFor(wsID) != nil {
		out = append(out, authz.Tuple{User: user, Relation: "grantee", Object: "workspace:" + wsID})
	}
	return out
}

// relation maps a configured tenant role to the model: owner, a manager role → admin, else member.
func (x shadowCtx) relation(role string) string {
	switch {
	case role == x.roles.Owner:
		return "owner"
	case slices.Contains(x.roles.Managers, role):
		return "admin"
	}
	return "member"
}
