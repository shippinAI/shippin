package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Membership is one Nomen membership of the caller.
type Membership struct {
	OrgID       string   `json:"orgId"`
	IAM         bool     `json:"iam"`
	Roles       []string `json:"roles"`
	DisplayName string   `json:"displayName"`
}

// Identity is what the IdP says about the caller.
type Identity struct {
	Sub           string
	Name          string
	Email         string
	EmailVerified bool
	OrgID         string
	OrgName       string
	Memberships   []Membership
	// ProjectRoles are the Nomen project roles from the role claims, with the orgs that granted each.
	ProjectRoles map[string][]string
	Tenant       string
}

func (f *Flow) call(ctx context.Context, method, path, tok string, body []byte, out any) error {
	return f.callOrg(ctx, method, path, tok, "", body, out)
}

// callOrg is call in the context of one IdP org (idp.OrgHeader); org "" = the token's own.
func (f *Flow) callOrg(ctx context.Context, method, path, tok, org string, body []byte, out any) error {
	url := strings.TrimRight(f.cfg.IDP.Issuer, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	if org != "" {
		req.Header.Set(f.cfg.IDP.OrgHeader(), org)
	}
	res, err := f.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("idp %s: %d", path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Whoami validates the token at the IdP and loads memberships (cached briefly).
func (f *Flow) Whoami(ctx context.Context, tok string) (*Identity, error) {
	if id := f.cache.get(tok); id != nil {
		return id, nil
	}
	id, err := f.whoami(ctx, tok)
	if err == nil {
		f.cache.put(tok, id)
	}
	return id, err
}

func (f *Flow) whoami(ctx context.Context, tok string) (*Identity, error) {
	var ui map[string]any
	if err := f.call(ctx, http.MethodGet, "/oidc/v1/userinfo", tok, nil, &ui); err != nil {
		return nil, err
	}
	s := func(k string) string { v, _ := ui[k].(string); return v }
	id := &Identity{Sub: s("sub"), Name: s("name"), Email: s("email"),
		OrgID: s(f.cfg.IDP.URN("user:resourceowner:id")), OrgName: s(f.cfg.IDP.URN("user:resourceowner:name"))}
	id.EmailVerified, _ = ui["email_verified"].(bool)
	if id.Name == "" {
		id.Name = s("preferred_username")
	}
	id.ProjectRoles = ProjectRoles(ui, f.roleClaims())
	id.Tenant = s(f.cfg.Identity.TenantClaim)
	var ms struct {
		Result []Membership `json:"result"`
	}
	if err := f.call(ctx, http.MethodPost, "/auth/v1/memberships/me/_search", tok, []byte(`{}`), &ms); err == nil {
		id.Memberships = ms.Result
	}
	return id, nil
}

func (f *Flow) roleClaims() []string {
	out := []string{}
	for _, c := range f.cfg.Identity.RoleClaims {
		out = append(out, strings.ReplaceAll(c, "{project}", f.cfg.Staff.ProjectID))
	}
	return out
}

// ProjectRoles reads Nomen role claims ({role: {orgId: domain}}) into role -> granting orgs.
func ProjectRoles(claims map[string]any, names []string) map[string][]string {
	out := map[string][]string{}
	for _, n := range names {
		m, _ := claims[n].(map[string]any)
		for role, v := range m {
			orgs, _ := v.(map[string]any)
			for o := range orgs {
				out[role] = append(out[role], o)
			}
		}
	}
	return out
}
