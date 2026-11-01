package api

import (
	"context"
	"strings"
)

type zUser struct {
	UserID   string `json:"userId"`
	State    string `json:"state"`
	Username string `json:"username"`
	Login    string `json:"preferredLoginName"`
	Details  struct {
		ResourceOwner string `json:"resourceOwner"`
	} `json:"details"`
	Human *struct {
		Profile struct {
			DisplayName string `json:"displayName"`
			GivenName   string `json:"givenName"`
			FamilyName  string `json:"familyName"`
		} `json:"profile"`
		Email struct {
			Email string `json:"email"`
		} `json:"email"`
	} `json:"human"`
	Machine *struct {
		Name string `json:"name"`
	} `json:"machine"`
}

func (u zUser) view() staffUser {
	s := staffUser{ID: u.UserID, Login: u.Login, Kind: "machine",
		State: strings.ToLower(strings.TrimPrefix(u.State, "USER_STATE_"))}
	if s.Login == "" {
		s.Login = u.Username
	}
	if u.Human != nil {
		s.Kind, s.Email = "human", u.Human.Email.Email
		s.Name = u.Human.Profile.DisplayName
		if s.Name == "" {
			s.Name = strings.TrimSpace(u.Human.Profile.GivenName + " " + u.Human.Profile.FamilyName)
		}
	} else if u.Machine != nil {
		s.Name = u.Machine.Name
	}
	if s.Name == "" {
		s.Name = s.Login
	}
	return s
}

func (z *nomenStaff) Users(ctx context.Context) ([]staffUser, error) {
	var res struct {
		Result []zUser `json:"result"`
	}
	body := map[string]any{"query": map[string]any{"limit": 500},
		"queries": []any{map[string]any{"organizationIdQuery": map[string]string{"organizationId": z.org}}}}
	if err := z.do(ctx, "POST", "/v2/users", body, &res); err != nil {
		return nil, err
	}
	out := []staffUser{}
	for _, u := range res.Result {
		if u.Details.ResourceOwner == z.org {
			out = append(out, u.view())
		}
	}
	return out, nil
}

// User returns a user of the operator org, or nil for anyone else (customers can't be targeted).
func (z *nomenStaff) User(ctx context.Context, id string) (*staffUser, error) {
	var res struct {
		User zUser `json:"user"`
	}
	if err := z.do(ctx, "GET", "/v2/users/"+id, nil, &res); err != nil {
		return nil, err
	}
	if res.User.Details.ResourceOwner != z.org {
		return nil, nil
	}
	v := res.User.view()
	return &v, nil
}

// Invite creates a human without a password; Nomen e-mails them an initialization link.
func (z *nomenStaff) Invite(ctx context.Context, email, given, family string) (string, error) {
	var res struct {
		UserID string `json:"userId"`
	}
	err := z.do(ctx, "POST", "/management/v1/users/human", map[string]any{"userName": email,
		"profile": map[string]string{"firstName": given, "lastName": family, "displayName": strings.TrimSpace(given + " " + family)},
		"email":   map[string]any{"email": email, "isEmailVerified": false}}, &res)
	return res.UserID, err
}

func (z *nomenStaff) SetRoles(ctx context.Context, user string, g staffGrant, roles []string) error {
	base := "/management/v1/users/" + user + "/grants"
	switch {
	case g.ID == "" && len(roles) == 0:
		return nil
	case g.ID == "":
		return z.do(ctx, "POST", base, map[string]any{"projectId": z.project, "roleKeys": roles}, nil)
	case len(roles) == 0:
		return z.do(ctx, "DELETE", base+"/"+g.ID, nil, nil)
	}
	return z.do(ctx, "PUT", base+"/"+g.ID, map[string]any{"roleKeys": roles}, nil)
}

func (z *nomenStaff) SetActive(ctx context.Context, user string, active bool) error {
	op := "/_deactivate"
	if active {
		op = "/_reactivate"
	}
	return z.do(ctx, "POST", "/management/v1/users/"+user+op, map[string]any{}, nil)
}
