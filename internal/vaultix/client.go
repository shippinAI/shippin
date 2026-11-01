// Package vaultix talks to Infisical (the Vaultix backend, 03 decision 16) as one Universal
// Auth machine identity. It is shared by the console and the keyring CLI. Secret values are
// only decoded by the calls that exist to return them (Reveal, Pull) and are never logged.
package vaultix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is a missing project, environment or secret.
var ErrNotFound = errors.New("vaultix: not found")

// Client is an Infisical API client with a cached, auto-renewed access token.
type Client struct {
	Base, ClientID, ClientSecret string
	HTTP                         *http.Client
	mu                           sync.Mutex
	token                        string
	until                        time.Time
}

// New builds a client for base (e.g. https://secrets.example.com).
func New(base, id, secret string, timeout time.Duration) *Client {
	return &Client{Base: strings.TrimRight(base, "/"), ClientID: id, ClientSecret: secret, HTTP: &http.Client{Timeout: timeout}}
}

func (c *Client) auth(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.until) {
		return c.token, nil
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return "", errors.New("vaultix: no machine identity configured")
	}
	var out struct {
		AccessToken string `json:"accessToken"`
		ExpiresIn   int    `json:"expiresIn"`
	}
	in := map[string]string{"clientId": c.ClientID, "clientSecret": c.ClientSecret}
	if err := c.do(ctx, "", http.MethodPost, "/api/v1/auth/universal-auth/login", in, &out); err != nil {
		return "", fmt.Errorf("vaultix login: %w", err)
	}
	ttl := time.Duration(out.ExpiresIn) * time.Second
	c.token, c.until = out.AccessToken, time.Now().Add(ttl*3/4)
	return c.token, nil
}

// call is do with the identity's bearer token.
func (c *Client) call(ctx context.Context, method, path string, in, out any) error {
	tok, err := c.auth(ctx)
	if err != nil {
		return err
	}
	return c.do(ctx, tok, method, path, in, out)
}

func (c *Client) do(ctx context.Context, tok, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 300:
		// Infisical error bodies name the problem, not values; still, keep only the message.
		var e struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		return fmt.Errorf("vaultix %s %s: %s %s", method, strings.SplitN(path, "?", 2)[0], resp.Status, e.Message)
	case out != nil:
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
