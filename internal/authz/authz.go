// Package authz is the one Authorize interface in front of OpenFGA (02a RBAC). Callers pass
// the subject, relation and object, plus contextual tuples for facts that live in the IdP
// token (org roles) or in Postgres (access grants), so those need no tuple sync.
package authz

import "context"

// Tuple is one OpenFGA relationship, e.g. {"user:<sub>", "admin", "tenant:<uuid>"}.
type Tuple struct {
	User     string `json:"user"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

// Check asks whether User has Relation on Object, given Context.
type Check struct {
	Tuple
	Context []Tuple
}

// Authorizer decides one check. Implementations must fail closed (false plus an error).
type Authorizer interface {
	Authorize(ctx context.Context, c Check) (bool, error)
}
