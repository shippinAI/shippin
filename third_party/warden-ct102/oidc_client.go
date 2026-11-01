package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type oidcProviderMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IDToken     string `json:"id_token"`
}

type oidcUserInfo struct {
	Subject           string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Name              string `json:"name"`
}

func fetchOIDCMetadata(ctx context.Context, cfg OIDCConfig) (oidcProviderMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.DiscoveryURL, nil)
	if err != nil {
		return oidcProviderMetadata{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oidcProviderMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oidcProviderMetadata{}, fmt.Errorf("metadata status %d", resp.StatusCode)
	}
	var metadata oidcProviderMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return oidcProviderMetadata{}, err
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" || metadata.UserInfoEndpoint == "" {
		return oidcProviderMetadata{}, errors.New("metadata missing endpoints")
	}
	return metadata, nil
}

func exchangeOIDCCode(ctx context.Context, cfg OIDCConfig, metadata oidcProviderMetadata, state oidcState, code string) (oidcTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", state.RedirectURL)
	form.Set("code_verifier", state.CodeVerifier)
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oidcTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oidcTokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return oidcTokenResponse{}, fmt.Errorf("token status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var token oidcTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return oidcTokenResponse{}, err
	}
	if token.AccessToken == "" {
		return oidcTokenResponse{}, errors.New("token response missing access token")
	}
	return token, nil
}

func (u oidcUserInfo) Username() string {
	for _, candidate := range []string{u.PreferredUsername, u.Email, u.Name, u.Subject} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func fetchOIDCUserInfo(ctx context.Context, metadata oidcProviderMetadata, accessToken string) (oidcUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadata.UserInfoEndpoint, nil)
	if err != nil {
		return oidcUserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oidcUserInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oidcUserInfo{}, fmt.Errorf("userinfo status %d", resp.StatusCode)
	}
	var info oidcUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return oidcUserInfo{}, err
	}
	return info, nil
}
