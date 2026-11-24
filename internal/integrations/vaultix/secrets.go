package vaultix

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Meta is a secret without its value.
type Meta struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type rawMeta struct {
	ID        string    `json:"id"`
	Key       string    `json:"secretKey"`
	Path      string    `json:"secretPath"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (r rawMeta) meta(path string) Meta {
	if r.Path == "" {
		r.Path = path
	}
	return Meta{ID: r.ID, Key: r.Key, Path: r.Path, Version: r.Version, UpdatedAt: r.UpdatedAt}
}

// Scope addresses one folder of one environment of one project.
type Scope struct{ Project, Env, Path string }

func (s Scope) query() url.Values {
	p := s.Path
	if p == "" {
		p = "/"
	}
	return url.Values{"workspaceId": {s.Project}, "environment": {s.Env}, "secretPath": {p}}
}

// List returns metadata only: the response struct has no value field, so values are dropped
// while decoding and never held.
func (c *Client) List(ctx context.Context, s Scope) ([]Meta, error) {
	var out struct {
		Secrets []rawMeta `json:"secrets"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/v3/secrets/raw?"+s.query().Encode(), nil, &out); err != nil {
		return nil, err
	}
	items := make([]Meta, 0, len(out.Secrets))
	for _, r := range out.Secrets {
		items = append(items, r.meta(s.Path))
	}
	return items, nil
}

// Reveal returns one secret's value. Callers audit before calling and never log the value.
func (c *Client) Reveal(ctx context.Context, s Scope, key string) (Meta, string, error) {
	var out struct {
		Secret struct {
			rawMeta
			Value string `json:"secretValue"`
		} `json:"secret"`
	}
	path := "/api/v3/secrets/raw/" + url.PathEscape(key) + "?" + s.query().Encode()
	if err := c.call(ctx, http.MethodGet, path, nil, &out); err != nil {
		return Meta{}, "", err
	}
	return out.Secret.meta(s.Path), out.Secret.Value, nil
}

// Pull returns every key and value in scope, for the keyring CLI's sync and run only.
func (c *Client) Pull(ctx context.Context, s Scope) (map[string]string, error) {
	var out struct {
		Secrets []struct {
			Key   string `json:"secretKey"`
			Value string `json:"secretValue"`
		} `json:"secrets"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/v3/secrets/raw?"+s.query().Encode(), nil, &out); err != nil {
		return nil, err
	}
	m := make(map[string]string, len(out.Secrets))
	for _, s := range out.Secrets {
		m[s.Key] = s.Value
	}
	return m, nil
}

// Set creates (create=true) or updates key; the response carries metadata only.
func (c *Client) Set(ctx context.Context, s Scope, key, value string, create bool) (Meta, error) {
	p := s.Path
	if p == "" {
		p = "/"
	}
	in := map[string]string{"workspaceId": s.Project, "environment": s.Env, "secretPath": p, "secretValue": value, "type": "shared"}
	method := http.MethodPatch
	if create {
		method = http.MethodPost
	}
	var out struct {
		Secret rawMeta `json:"secret"`
	}
	err := c.call(ctx, method, "/api/v3/secrets/raw/"+url.PathEscape(key), in, &out)
	return out.Secret.meta(p), err
}

// Delete removes key from scope.
func (c *Client) Delete(ctx context.Context, s Scope, key string) error {
	p := s.Path
	if p == "" {
		p = "/"
	}
	in := map[string]string{"workspaceId": s.Project, "environment": s.Env, "secretPath": p, "type": "shared"}
	return c.call(ctx, http.MethodDelete, "/api/v3/secrets/raw/"+url.PathEscape(key), in, nil)
}
