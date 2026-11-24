package vaultix

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

// Operator is the platform's Vaultix instance admin (a person account, email + password login).
// Only it can create organizations; it hands each one to a per-tenant machine identity at once.
type Operator struct {
	c               *Client
	email, password string
	mu              sync.Mutex
	tok             string
	until           time.Time
}

// NewOperator logs in lazily; credentials come from the secrets store, never from config.
func NewOperator(base, email, password string, timeout time.Duration) *Operator {
	return &Operator{c: New(base, "", "", timeout), email: email, password: password}
}

func (o *Operator) jwt(ctx context.Context) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.tok != "" && time.Now().Before(o.until) {
		return o.tok, nil
	}
	if o.email == "" || o.password == "" {
		return "", errors.New("vaultix: no operator login configured")
	}
	var out struct {
		AccessToken string `json:"accessToken"`
	}
	if err := o.c.do(ctx, "", http.MethodPost, "/api/v3/auth/login", map[string]string{"email": o.email, "password": o.password}, &out); err != nil {
		return "", err
	}
	o.tok, o.until = out.AccessToken, time.Now().Add(10*time.Minute)
	return o.tok, nil
}

// orgJWT is the operator's token scoped to org (Infisical tokens carry one organization).
func (o *Operator) orgJWT(ctx context.Context, org string) (string, error) {
	tok, err := o.jwt(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		Token string `json:"token"`
	}
	err = o.c.do(ctx, tok, http.MethodPost, "/api/v3/auth/select-organization", map[string]string{"organizationId": org}, &out)
	return out.Token, err
}

// CreateOrg makes an organization owned by the operator and returns its id.
func (o *Operator) CreateOrg(ctx context.Context, name string) (string, error) {
	tok, err := o.jwt(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	err = o.c.do(ctx, tok, http.MethodPost, "/api/v2/organizations", map[string]string{"name": name}, &out)
	return out.Organization.ID, err
}

// CreateIdentity adds an org-admin machine identity with Universal Auth and returns ids.
func (o *Operator) CreateIdentity(ctx context.Context, org, name string) (id, clientID string, err error) {
	tok, err := o.orgJWT(ctx, org)
	if err != nil {
		return "", "", err
	}
	var idn struct {
		Identity struct {
			ID string `json:"id"`
		} `json:"identity"`
	}
	in := map[string]string{"name": name, "organizationId": org, "role": "admin"}
	if err := o.c.do(ctx, tok, http.MethodPost, "/api/v1/identities", in, &idn); err != nil {
		return "", "", err
	}
	var ua struct {
		UA struct {
			ClientID string `json:"clientId"`
		} `json:"identityUniversalAuth"`
	}
	err = o.c.do(ctx, tok, http.MethodPost, "/api/v1/auth/universal-auth/identities/"+idn.Identity.ID, map[string]any{}, &ua)
	return idn.Identity.ID, ua.UA.ClientID, err
}

// MintSecret creates a new client secret for identity (returned once; Vaultix keeps only a hash).
func (o *Operator) MintSecret(ctx context.Context, org, identity, label string) (string, error) {
	tok, err := o.orgJWT(ctx, org)
	if err != nil {
		return "", err
	}
	var out struct {
		ClientSecret string `json:"clientSecret"`
	}
	path := "/api/v1/auth/universal-auth/identities/" + identity + "/client-secrets"
	err = o.c.do(ctx, tok, http.MethodPost, path, map[string]string{"description": label}, &out)
	return out.ClientSecret, err
}
