package auth

import (
	"context"
	"encoding/json"
	"errors"
)

// CreateMachineUser creates an IdP service user (no password, no key, no token) in org with the
// console's service token, and returns its user id. Used for agent identities.
func (f *Flow) CreateMachineUser(ctx context.Context, svcTok, org, userName, name, description string) (string, error) {
	if svcTok == "" || org == "" {
		return "", errors.New("no IdP service token or org")
	}
	body, _ := json.Marshal(map[string]any{"userName": userName, "name": name, "description": description,
		"accessTokenType": "ACCESS_TOKEN_TYPE_JWT"})
	var out struct {
		UserID string `json:"userId"`
	}
	if err := f.callOrg(ctx, "POST", "/management/v1/users/machine", svcTok, org, body, &out); err != nil {
		return "", err
	}
	if out.UserID == "" {
		return "", errors.New("idp returned no user id")
	}
	return out.UserID, nil
}
