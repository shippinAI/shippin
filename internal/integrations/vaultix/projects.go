package vaultix

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// CreateProject makes a secret-manager project and returns its id. The identity becomes its admin.
func (c *Client) CreateProject(ctx context.Context, name, slug string) (string, error) {
	var out struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	in := map[string]any{"projectName": name, "slug": slug, "type": "secret-manager"}
	if err := c.call(ctx, http.MethodPost, "/api/v2/workspace", in, &out); err != nil {
		return "", err
	}
	return out.Project.ID, nil
}

// Envs returns the environment slugs a project has.
func (c *Client) Envs(ctx context.Context, project string) ([]string, error) {
	var out struct {
		Workspace struct {
			Environments []struct {
				Slug string `json:"slug"`
			} `json:"environments"`
		} `json:"workspace"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/v1/workspace/"+project, nil, &out); err != nil {
		return nil, err
	}
	var s []string
	for _, e := range out.Workspace.Environments {
		s = append(s, e.Slug)
	}
	return s, nil
}

// EnsureEnvs adds any of want the project lacks (named after the slug, capitalised).
func (c *Client) EnsureEnvs(ctx context.Context, project string, want []string) error {
	have, err := c.Envs(ctx, project)
	if err != nil {
		return err
	}
	for _, w := range want {
		if contains(have, w) {
			continue
		}
		in := map[string]string{"name": strings.ToUpper(w[:1]) + w[1:], "slug": w}
		if err := c.call(ctx, http.MethodPost, "/api/v1/workspace/"+project+"/environments", in, nil); err != nil {
			return err
		}
	}
	return nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ProjectBySlug returns the id of the project with slug (tenant projects are <prefix><tenant>).
func (c *Client) ProjectBySlug(ctx context.Context, slug string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/v2/workspace/"+url.PathEscape(slug), nil, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// AddIdentity makes identity a project member with role (e.g. "member"); the caller must be
// a project admin.
func (c *Client) AddIdentity(ctx context.Context, project, identity, role string) error {
	in := map[string]any{"roles": []map[string]string{{"role": role}}}
	return c.call(ctx, http.MethodPost, "/api/v2/workspace/"+project+"/identity-memberships/"+identity, in, nil)
}
