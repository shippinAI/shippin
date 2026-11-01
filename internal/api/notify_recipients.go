package api

import (
	"context"
	"slices"
	"strings"
)

// managerEmails returns the e-mail addresses of a tenant org's members whose configured role is
// a manager role (owners/admins). Machine users have none and are skipped.
func (a *API) managerEmails(ctx context.Context, org string) ([]string, error) {
	var res struct {
		Result []struct {
			Email              string   `json:"email"`
			PreferredLoginName string   `json:"preferredLoginName"`
			Roles              []string `json:"roles"`
		} `json:"result"`
	}
	if err := a.flow.IdPRead(ctx, a.cfg.IDPReadToken, "POST", "/management/v1/orgs/me/members/_search", org, map[string]any{}, &res); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range res.Result {
		manager := slices.ContainsFunc(m.Roles, func(r string) bool { return slices.Contains(a.cfg.Roles.Managers, a.orgRole(r)) })
		if manager && strings.Contains(m.Email, "@") {
			out = append(out, m.Email)
		}
	}
	return out, nil
}

// userEmail returns a user's verified-or-not e-mail address ("" for machine users).
func (a *API) userEmail(ctx context.Context, sub string) (string, error) {
	var res struct {
		User struct {
			Human *struct {
				Email struct{ Email string } `json:"email"`
			} `json:"human"`
		} `json:"user"`
	}
	if !uuidOrDigits(sub) {
		return "", nil
	}
	if err := a.flow.IdPRead(ctx, a.cfg.IDPReadToken, "GET", "/v2/users/"+sub, "", nil, &res); err != nil {
		return "", err
	}
	if res.User.Human == nil {
		return "", nil
	}
	return res.User.Human.Email.Email, nil
}

func uuidOrDigits(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef-") == ""
}
