package api

import (
	"context"

	"shippin.cloud/console/internal/identity/ssh_key"
)

// userKeys are a person's public keys, for the workspaces and jobs this API starts. Adding and
// removing keys are actions in internal/identity/ssh_key, and its PushToWorkspaces reaction keeps
// their workspaces current through ports.Workspaces.
func (a *API) userKeys(ctx context.Context, sub string) ([]string, error) {
	return ssh_key.PublicKeys(ctx, a.db.Pool, sub)
}
