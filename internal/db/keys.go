package db

import "context"

// WorkspacesByCreator lists live workspaces created by a user (to push key changes).
func (d *DB) WorkspacesByCreator(ctx context.Context, sub string) ([]Workspace, error) {
	return d.queryWS(ctx, `WHERE created_by=$1 AND state<>'deleted'`, sub)
}
