package vaultix

import (
	"context"
	"net/http"
	"strings"
)

// EnsureFolder creates every missing segment of path (e.g. /workspaces/<id>/connect) in env.
// An "already exists" answer counts as success.
func (c *Client) EnsureFolder(ctx context.Context, project, env, path string) error {
	parent := "/"
	for _, name := range strings.Split(strings.Trim(path, "/"), "/") {
		if name == "" {
			continue
		}
		in := map[string]string{"workspaceId": project, "environment": env, "path": parent, "name": name}
		if err := c.call(ctx, http.MethodPost, "/api/v1/folders", in, nil); err != nil && !strings.Contains(strings.ToLower(err.Error()), "exist") {
			return err
		}
		parent = strings.TrimSuffix(parent, "/") + "/" + name
	}
	return nil
}
