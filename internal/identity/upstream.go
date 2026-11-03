package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"

	"shippin.cloud/console/internal/config"
)

// Discovery is the subset of an OpenID provider configuration this directory uses.
type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// Discover loads the issuer's published endpoints. HTTP must be non-nil.
func Discover(ctx context.Context, client *http.Client, issuer string) (Discovery, error) {
	issuer = strings.TrimRight(issuer, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return Discovery{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return Discovery{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Discovery{}, errors.New("discovery failed")
	}
	var d Discovery
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&d); err != nil {
		return Discovery{}, err
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.UserinfoEndpoint == "" {
		return Discovery{}, errors.New("discovery document is incomplete")
	}
	return d, nil
}

// UpstreamAuthURL is the PKCE redirect for one configured issuer.
func UpstreamAuthURL(d Discovery, u config.Upstream, redirect, state, verifier string) string {
	oa := oauth2.Config{
		ClientID:    u.ClientID,
		Endpoint:    oauth2.Endpoint{AuthURL: d.AuthorizationEndpoint, TokenURL: d.TokenEndpoint},
		RedirectURL: redirect,
		Scopes:      []string{"openid", "profile", "email"},
	}
	return oa.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

// UpstreamIdentity is the verified subject an issuer returned.
type UpstreamIdentity struct {
	Issuer        string
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
}

// ExchangeUpstream trades a code and reads userinfo.
func ExchangeUpstream(ctx context.Context, client *http.Client, d Discovery, u config.Upstream, redirect, code, verifier, secret string) (UpstreamIdentity, error) {
	oa := oauth2.Config{
		ClientID:     u.ClientID,
		ClientSecret: secret,
		Endpoint:     oauth2.Endpoint{AuthURL: d.AuthorizationEndpoint, TokenURL: d.TokenEndpoint, AuthStyle: oauth2.AuthStyleInParams},
		RedirectURL:  redirect,
		Scopes:       []string{"openid", "profile", "email"},
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	tok, err := oa.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return UpstreamIdentity{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.UserinfoEndpoint, nil)
	if err != nil {
		return UpstreamIdentity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	res, err := client.Do(req)
	if err != nil {
		return UpstreamIdentity{}, err
	}
	defer res.Body.Close()
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil || info.Sub == "" {
		return UpstreamIdentity{}, errors.New("userinfo")
	}
	return UpstreamIdentity{Issuer: strings.TrimRight(u.Issuer, "/"), Sub: info.Sub, Email: info.Email, EmailVerified: info.EmailVerified, Name: info.Name}, nil
}

// FindUpstream returns a configured upstream by id.
func FindUpstream(cfg *config.Config, id string) (config.Upstream, bool) {
	for _, u := range cfg.Directory.Upstreams {
		if u.ID == id && u.Configured() {
			if u.Label == "" {
				u.Label = u.ID
			}
			return u, true
		}
	}
	if id == "nomen" && cfg.IDP.Issuer != "" && cfg.IDP.ClientID != "" {
		return config.Upstream{ID: "nomen", Label: cfg.IDP.Name, Issuer: cfg.IDP.Issuer, ClientID: cfg.IDP.ClientID, SecretEnv: "SHIPPIN_OIDC_CLIENT_SECRET"}, true
	}
	return config.Upstream{}, false
}

// RedirectFor builds the callback this console advertises. base is the public origin.
func RedirectFor(base string) string {
	return strings.TrimRight(base, "/") + "/auth/upstream/callback"
}

func secretFromEnv(getenv func(string) string, name string) string {
	if name == "" {
		return ""
	}
	return getenv(name)
}

// QueryEscape is used by tests that build discovery URLs.
func QueryEscape(s string) string { return url.QueryEscape(s) }
