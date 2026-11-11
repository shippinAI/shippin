package api

import (
	"context"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// localCaller resolves a local directory session into the same caller an IdP token produces.
// The user's directory orgs become org memberships, and the installation owner holds the
// platform admin role (operator.role), so the first owner can run the panel without an IdP.
func (a *API) localCaller(r *http.Request, uid string) (*caller, error) {
	id, err := a.localIdentity(r.Context(), uid)
	if err != nil {
		return nil, errIdP
	}
	return a.callerFromIdentity(r, id)
}

// localSubPrefix keeps local subjects apart from any IdP subject.
const localSubPrefix = "local:"

// LocalSubject is the staff subject of a local directory user, as roles and direct grants name it.
func LocalSubject(userID string) string { return localSubPrefix + userID }

func (a *API) localIdentity(ctx context.Context, uid string) (*auth.Identity, error) {
	if a.directory == nil || a.db == nil {
		return nil, errIdP
	}
	var email, name, role string
	var verified bool
	if err := a.db.Pool.QueryRow(ctx, `SELECT email, email_verified, name, role FROM identity_users WHERE id = $1 AND active`, uid).
		Scan(&email, &verified, &name, &role); err != nil {
		return nil, err
	}
	id := &auth.Identity{Sub: localSubPrefix + uid, Name: name, Email: email, EmailVerified: verified}
	if id.Name == "" {
		id.Name = email
	}
	rows, err := a.db.Pool.Query(ctx, `SELECT org_id::text, role FROM identity_members WHERE user_id = $1 ORDER BY org_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var org, orole string
		if err := rows.Scan(&org, &orole); err != nil {
			return nil, err
		}
		if id.OrgID == "" {
			id.OrgID = org
		}
		id.Memberships = append(id.Memberships, auth.Membership{OrgID: org, Roles: []string{localOrgRole(orole)}})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if iam := a.localIAMRole(role); iam != "" {
		id.Memberships = append(id.Memberships, auth.Membership{IAM: true, Roles: []string{iam}})
	}
	return id, nil
}

// localOrgRole names a directory org role the way roles.org expects (ORG_OWNER -> org_owner).
func localOrgRole(r string) string {
	switch r {
	case "owner":
		return "ORG_OWNER"
	case "admin":
		return "ORG_ADMIN"
	}
	return "ORG_MEMBER"
}

// localIAMRole: only the installation owner is a platform admin. It is the roles.iam source that
// maps to operator.role, so the mapping stays in config. Admins and members get no operator role.
func (a *API) localIAMRole(userRole string) string {
	if userRole != "owner" {
		return ""
	}
	for _, m := range a.cfg.Roles.IAM {
		if m.To == a.cfg.Operator.Role {
			return m.From
		}
	}
	return ""
}
