package networking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

type providerURLs struct {
	cloudflare     string
	tailscale      string
	tailscaleToken string
}

var defaultProviderURLs = providerURLs{
	cloudflare:     "https://api.cloudflare.com/client/v4",
	tailscale:      "https://api.tailscale.com/api/v2",
	tailscaleToken: "https://api.tailscale.com/api/v2/oauth/token",
}

// Inventory calls the configured provider's real read API. Credentials are supplied only for the
// duration of the request and never appear in returned resources or provider error text.
func Inventory(ctx context.Context, c Connection, credential []byte) ([]Resource, error) {
	return inventory(ctx, c, credential, &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, defaultProviderURLs)
}

func inventory(ctx context.Context, c Connection, credential []byte, hc *http.Client, endpoints providerURLs) ([]Resource, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return nil, errors.New("invalid provider configuration")
	}
	switch c.Provider {
	case CloudflareTunnel:
		return cloudflareInventory(ctx, hc, c, cfg, string(credential), endpoints.cloudflare)
	case Tailscale:
		return tailscaleInventory(ctx, hc, c, cfg, string(credential), endpoints.tailscale, endpoints.tailscaleToken)
	case Custom:
		return customInventory(ctx, hc, c, cfg, string(credential))
	default:
		return nil, errors.New("provider is unsupported")
	}
}

func cloudflareInventory(ctx context.Context, hc *http.Client, c Connection, cfg Config, token, baseURL string) ([]Resource, error) {
	if token == "" {
		return nil, errors.New("Cloudflare API token is not configured")
	}
	var out struct {
		Success bool `json:"success"`
		Result  []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
			DeletedAt string `json:"deleted_at"`
		} `json:"result"`
		ResultInfo struct {
			TotalPages int `json:"total_pages"`
		} `json:"result_info"`
	}
	items := []Resource{}
	for page := 1; page <= 20; page++ {
		query := url.Values{"is_deleted": {"false"}, "page": {fmt.Sprint(page)}, "per_page": {"100"}}
		target := strings.TrimRight(baseURL, "/") + "/accounts/" + url.PathEscape(cfg.AccountID) + "/cfd_tunnel?" + query.Encode()
		b, err := request(ctx, hc, http.MethodGet, target, "Bearer "+token, nil)
		if err != nil {
			return nil, err
		}
		if json.Unmarshal(b, &out) != nil || !out.Success {
			return nil, errors.New("Cloudflare returned an invalid tunnel inventory")
		}
		for _, v := range out.Result {
			if v.ID == "" {
				continue
			}
			details, _ := json.Marshal(map[string]string{"created_at": v.CreatedAt, "deleted_at": v.DeletedAt})
			items = append(items, Resource{ID: v.ID, ConnectionID: c.ID, Provider: c.Provider, Kind: "tunnel", Name: v.Name, State: v.Status, Details: details, ObservedAt: time.Now().UTC()})
		}
		if page >= max(1, out.ResultInfo.TotalPages) {
			return items, nil
		}
	}
	return nil, errors.New("Cloudflare inventory exceeds 2000 tunnels")
}

func tailscaleInventory(ctx context.Context, hc *http.Client, c Connection, cfg Config, secret, baseURL, tokenURL string) ([]Resource, error) {
	if secret == "" {
		return nil, errors.New("Tailscale OAuth secret is not configured")
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {cfg.ClientID}, "client_secret": {secret}, "scope": {"devices:core:read"}}
	b, err := request(ctx, hc, http.MethodPost, tokenURL, "application/x-www-form-urlencoded", []byte(form.Encode()))
	if err != nil {
		return nil, err
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(b, &tok) != nil || tok.AccessToken == "" {
		return nil, errors.New("Tailscale OAuth returned no access token")
	}
	u := strings.TrimRight(baseURL, "/") + "/tailnet/" + url.PathEscape(cfg.Tailnet) + "/devices"
	b, err = request(ctx, hc, http.MethodGet, u, "Bearer "+tok.AccessToken, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Devices []struct {
			ID            string   `json:"id"`
			Name          string   `json:"name"`
			Hostname      string   `json:"hostname"`
			Addresses     []string `json:"addresses"`
			OS            string   `json:"os"`
			ClientVersion string   `json:"clientVersion"`
			LastSeen      string   `json:"lastSeen"`
			Expired       bool     `json:"expired"`
			Authorized    bool     `json:"authorized"`
		} `json:"devices"`
	}
	if json.Unmarshal(b, &out) != nil {
		return nil, errors.New("Tailscale returned an invalid device inventory")
	}
	now := time.Now().UTC()
	items := make([]Resource, 0, len(out.Devices))
	for _, v := range out.Devices {
		if v.ID == "" {
			continue
		}
		state := "active"
		if v.Expired {
			state = "expired"
		} else if !v.Authorized {
			state = "unauthorized"
		}
		address := ""
		if len(v.Addresses) > 0 {
			address = v.Addresses[0]
		}
		details, _ := json.Marshal(map[string]string{"hostname": v.Hostname, "os": v.OS, "client_version": v.ClientVersion, "last_seen": v.LastSeen})
		items = append(items, Resource{ID: v.ID, ConnectionID: c.ID, Provider: c.Provider, Kind: "device", Name: first(v.Name, v.Hostname), State: state, Address: address, Details: details, ObservedAt: now})
	}
	return items, nil
}

func customInventory(ctx context.Context, hc *http.Client, c Connection, cfg Config, token string) ([]Resource, error) {
	var auth string
	if token != "" {
		auth = "Bearer " + token
	}
	b, err := request(ctx, hc, http.MethodGet, cfg.InventoryURL, auth, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Name    string `json:"name"`
			State   string `json:"state"`
			Address string `json:"address"`
		} `json:"items"`
	}
	if json.Unmarshal(b, &out) != nil {
		return nil, errors.New("custom inventory must return an items array")
	}
	if len(out.Items) > 2000 {
		return nil, errors.New("custom inventory exceeds 2000 resources")
	}
	now := time.Now().UTC()
	items := make([]Resource, 0, len(out.Items))
	seen := map[string]bool{}
	for _, v := range out.Items {
		if v.ID == "" || v.Name == "" || v.Kind == "" || seen[v.ID] {
			return nil, errors.New("custom inventory contains a missing or duplicate resource identity")
		}
		seen[v.ID] = true
		items = append(items, Resource{ID: v.ID, ConnectionID: c.ID, Provider: c.Provider, Kind: v.Kind, Name: v.Name, State: v.State, Address: v.Address, Details: json.RawMessage(`{}`), ObservedAt: now})
	}
	return items, nil
}

func request(ctx context.Context, hc *http.Client, method, target, auth string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, r)
	if err != nil {
		return nil, errors.New("provider request could not be created")
	}
	if strings.HasPrefix(auth, "Bearer ") {
		req.Header.Set("Authorization", auth)
	} else if body != nil && auth == "application/x-www-form-urlencoded" {
		req.Header.Set("Content-Type", auth)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider request failed: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("provider response could not be read")
	}
	if len(b) > maxResponseBytes {
		return nil, errors.New("provider response exceeds 2 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	return b, nil
}

func first(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
