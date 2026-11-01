package api

import (
	"context"
	"errors"
)

var errNotOrgMember = errors.New("owner is not a member of the tenant's Nomen org")

// orgMember reports whether Nomen user sub belongs to org (its resource owner). Fails closed: a
// lookup error is "no". Test seam: a.orgOfUser.
func (a *API) orgMember(ctx context.Context, org, sub string) error {
	look := a.orgOfUser
	if look == nil {
		look = a.nomenOrgOf
	}
	got, err := look(ctx, sub)
	if err != nil || got == "" || got != org {
		return errNotOrgMember
	}
	return nil
}

// nomenOrgOf reads the user's organization from Nomen (v2 user API, read-only, team token).
func (a *API) nomenOrgOf(ctx context.Context, sub string) (string, error) {
	var out struct {
		User struct {
			Details struct {
				ResourceOwner string `json:"resourceOwner"`
			} `json:"details"`
		} `json:"user"`
	}
	if a.cfg.Staff.TeamToken == "" || !nomenSubRe.MatchString(sub) {
		return "", errNoTeamToken
	}
	err := a.flow.IdPRead(ctx, a.cfg.Staff.TeamToken, "GET", "/v2/users/"+sub, "", nil, &out)
	return out.User.Details.ResourceOwner, err
}
