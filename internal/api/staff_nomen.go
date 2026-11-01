package api

import (
	"context"
	"errors"
	"slices"

	"shippin.cloud/console/internal/auth"
)

// nomenStaff talks to Nomen as the console-team machine user, scoped to the operator org
// (x-nomen-orgid) and the staff project. It never uses an instance-level identity.
type nomenStaff struct {
	flow              *auth.Flow
	tok, org, project string
}

var errNoTeamToken = errors.New("staff: no team token or project configured")

func (z *nomenStaff) do(ctx context.Context, method, path string, body, out any) error {
	if z.tok == "" || z.project == "" || z.org == "" {
		return errNoTeamToken
	}
	if out == nil {
		out = &map[string]any{}
	}
	return z.flow.IdPRead(ctx, z.tok, method, path, z.org, body, out)
}

type zGrant struct {
	ID       string   `json:"id"`
	UserID   string   `json:"userId"`
	State    string   `json:"state"`
	RoleKeys []string `json:"roleKeys"`
	Project  string   `json:"projectId"`
	Details  struct {
		ResourceOwner string `json:"resourceOwner"`
	} `json:"details"`
}

func (z *nomenStaff) grants(ctx context.Context, user string) ([]zGrant, error) {
	qs := []any{map[string]any{"projectIdQuery": map[string]string{"projectId": z.project}}}
	if user != "" {
		qs = append(qs, map[string]any{"userIdQuery": map[string]string{"userId": user}})
	}
	var res struct {
		Result []zGrant `json:"result"`
	}
	err := z.do(ctx, "POST", "/management/v1/users/grants/_search", map[string]any{"query": map[string]any{"limit": 1000}, "queries": qs}, &res)
	out := []zGrant{}
	for _, g := range res.Result {
		// Only grants the operator org made on the staff project, and only active ones, count.
		if g.Project == z.project && g.Details.ResourceOwner == z.org && g.State != "USER_GRANT_STATE_INACTIVE" {
			out = append(out, g)
		}
	}
	return out, err
}

func (z *nomenStaff) Grant(ctx context.Context, user string) (staffGrant, error) {
	gs, err := z.grants(ctx, user)
	if err != nil || len(gs) == 0 {
		return staffGrant{}, err
	}
	return staffGrant{ID: gs[0].ID, Roles: gs[0].RoleKeys}, nil
}

func (z *nomenStaff) AllGrants(ctx context.Context) (map[string]staffGrant, error) {
	gs, err := z.grants(ctx, "")
	m := map[string]staffGrant{}
	for _, g := range gs {
		m[g.UserID] = staffGrant{ID: g.ID, Roles: g.RoleKeys}
	}
	return m, err
}

// mfaMethods are second factors; a password alone (or a PAT) never counts.
var mfaMethods = []string{"AUTHENTICATION_METHOD_TYPE_TOTP", "AUTHENTICATION_METHOD_TYPE_U2F",
	"AUTHENTICATION_METHOD_TYPE_PASSKEY", "AUTHENTICATION_METHOD_TYPE_OTP_SMS", "AUTHENTICATION_METHOD_TYPE_OTP_EMAIL"}

func (z *nomenStaff) MFA(ctx context.Context, user string) (bool, error) {
	var res struct {
		Types []string `json:"authMethodTypes"`
	}
	if err := z.do(ctx, "GET", "/v2/users/"+user+"/authentication_methods", nil, &res); err != nil {
		return false, err
	}
	for _, t := range res.Types {
		if slices.Contains(mfaMethods, t) {
			return true, nil
		}
	}
	return false, nil
}

func (z *nomenStaff) RoleKeys(ctx context.Context) ([]string, error) {
	var res struct {
		Result []struct {
			Key string `json:"key"`
		} `json:"result"`
	}
	err := z.do(ctx, "POST", "/management/v1/projects/"+z.project+"/roles/_search", map[string]any{"query": map[string]any{"limit": 1000}}, &res)
	keys := []string{}
	for _, r := range res.Result {
		keys = append(keys, r.Key)
	}
	return keys, err
}

func (z *nomenStaff) PutRole(ctx context.Context, key, name string, exists bool) error {
	if exists {
		return z.do(ctx, "PUT", "/management/v1/projects/"+z.project+"/roles/"+key, map[string]string{"displayName": name, "group": "staff"}, nil)
	}
	return z.do(ctx, "POST", "/management/v1/projects/"+z.project+"/roles", map[string]string{"roleKey": key, "displayName": name, "group": "staff"}, nil)
}

func (z *nomenStaff) DeleteRole(ctx context.Context, key string) error {
	return z.do(ctx, "DELETE", "/management/v1/projects/"+z.project+"/roles/"+key, nil, nil)
}
