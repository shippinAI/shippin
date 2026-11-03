package auth

import (
	"context"
	"encoding/json"
	"errors"
)

// Forget drops the cached identity for a token (e.g. after its memberships changed).
func (f *Flow) Forget(tok string) { f.cache.drop(tok) }

// CreateOrg creates a Nomen organization with ownerID as ORG_OWNER, using the console's
// service token (a machine user allowed to create orgs). Returns the new org id.
func (f *Flow) CreateOrg(ctx context.Context, svcTok, name, ownerID string) (string, error) {
	if svcTok == "" {
		return "", errors.New("no IdP service token configured")
	}
	body, _ := json.Marshal(map[string]any{
		"name":   name,
		"admins": []map[string]any{{"userId": ownerID, "roles": []string{"ORG_OWNER"}}},
	})
	var out struct {
		OrganizationID string `json:"organizationId"`
	}
	if err := f.call(ctx, "POST", "/v2/organizations", svcTok, body, &out); err != nil {
		return "", err
	}
	if out.OrganizationID == "" {
		return "", errors.New("idp returned no organization id")
	}
	return out.OrganizationID, nil
}
