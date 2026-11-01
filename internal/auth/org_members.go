package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
)

// Org membership writes in one tenant org (x-zitadel-orgid), with the console's service token.
// Nothing here logs or returns the token.

// OrgMember is a member of an org with its IdP org roles.
type OrgMember struct {
	UserID string   `json:"user_id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
}

func (f *Flow) orgDo(ctx context.Context, tok, org, method, path string, body, out any) error {
	if tok == "" {
		return errors.New("no IdP service token configured")
	}
	if org == "" {
		return errors.New("tenant has no IdP organization")
	}
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	if out == nil {
		out = &struct{}{}
	}
	return f.callOrg(ctx, method, path, tok, org, b, out)
}

// OrgMembers lists the org's members (limit 0 = IdP default).
func (f *Flow) OrgMembers(ctx context.Context, tok, org string, limit int) ([]OrgMember, error) {
	var res struct {
		Result []struct {
			UserID      string   `json:"userId"`
			DisplayName string   `json:"displayName"`
			Email       string   `json:"email"`
			Roles       []string `json:"roles"`
		} `json:"result"`
	}
	q := map[string]any{}
	if limit > 0 {
		q["limit"] = limit
	}
	if err := f.orgDo(ctx, tok, org, "POST", "/management/v1/orgs/me/members/_search", map[string]any{"query": q}, &res); err != nil {
		return nil, err
	}
	out := make([]OrgMember, 0, len(res.Result))
	for _, m := range res.Result {
		out = append(out, OrgMember{UserID: m.UserID, Name: m.DisplayName, Email: m.Email, Roles: m.Roles})
	}
	return out, nil
}

// UserByEmail returns the id of the user with exactly this e-mail anywhere in the instance, or "".
func (f *Flow) UserByEmail(ctx context.Context, tok, email string) (string, error) {
	var res struct {
		Result []struct {
			UserID string `json:"userId"`
		} `json:"result"`
	}
	body := map[string]any{"queries": []any{map[string]any{"emailQuery": map[string]any{"emailAddress": email, "method": "TEXT_QUERY_METHOD_EQUALS_IGNORE_CASE"}}}}
	if tok == "" {
		return "", errors.New("no IdP service token configured")
	}
	b, _ := json.Marshal(body)
	if err := f.callOrg(ctx, "POST", "/v2/users", tok, "", b, &res); err != nil {
		return "", err
	}
	if len(res.Result) != 1 {
		return "", nil
	}
	return res.Result[0].UserID, nil
}

// InviteHuman creates a human in org without a password; the IdP e-mails an initialization link.
func (f *Flow) InviteHuman(ctx context.Context, tok, org, email, given, family string) (string, error) {
	var res struct {
		UserID string `json:"userId"`
	}
	err := f.orgDo(ctx, tok, org, "POST", "/management/v1/users/human", map[string]any{"userName": email,
		"profile": map[string]string{"firstName": given, "lastName": family, "displayName": given + " " + family},
		"email":   map[string]any{"email": email, "isEmailVerified": false}}, &res)
	if err == nil && res.UserID == "" {
		err = errors.New("idp returned no user id")
	}
	return res.UserID, err
}

// SetOrgMember adds user to org with roles (add) or replaces the member's roles.
func (f *Flow) SetOrgMember(ctx context.Context, tok, org, user string, roles []string, add bool) error {
	if add {
		return f.orgDo(ctx, tok, org, "POST", "/management/v1/orgs/me/members", map[string]any{"userId": user, "roles": roles}, nil)
	}
	return f.orgDo(ctx, tok, org, "PUT", "/management/v1/orgs/me/members/"+url.PathEscape(user), map[string]any{"roles": roles}, nil)
}

// RemoveOrgMember removes user's membership of org (the user account itself is kept).
func (f *Flow) RemoveOrgMember(ctx context.Context, tok, org, user string) error {
	return f.orgDo(ctx, tok, org, "DELETE", "/management/v1/orgs/me/members/"+url.PathEscape(user), nil, nil)
}
