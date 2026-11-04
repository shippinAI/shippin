package networking

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderInventoryUsesScopedReadAPIs(t *testing.T) {
	t.Run("Cloudflare tunnel pagination and token scope", func(t *testing.T) {
		pages := 0
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pages++
			if r.Method != http.MethodGet || r.URL.Path != "/accounts/account-1/cfd_tunnel" || r.URL.Query().Get("per_page") != "100" || r.Header.Get("Authorization") != "Bearer cf-secret" {
				t.Fatalf("unexpected Cloudflare request: %s %s", r.Method, r.URL)
			}
			if pages == 1 {
				_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"t1","name":"edge-1","status":"healthy"}],"result_info":{"total_pages":2}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"t2","name":"edge-2","status":"inactive"}],"result_info":{"total_pages":2}}`))
		}))
		defer srv.Close()
		c := Connection{ID: "c1", Label: "cf", Provider: CloudflareTunnel, Config: json.RawMessage(`{"account_id":"account-1"}`), SyncIntervalSeconds: 300}
		items, err := inventory(context.Background(), c, []byte("cf-secret"), srv.Client(), providerURLs{cloudflare: srv.URL})
		if err != nil || pages != 2 || len(items) != 2 || items[1].ID != "t2" {
			t.Fatalf("items=%+v pages=%d err=%v", items, pages, err)
		}
	})

	t.Run("Tailscale requests read-only device scope", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oauth/token":
				if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("scope") != "devices:core:read" || r.Form.Get("client_secret") != "ts-secret" {
					t.Fatalf("unexpected OAuth request: %+v", r.Form)
				}
				_, _ = w.Write([]byte(`{"access_token":"short-lived"}`))
			case "/tailnet/example.com/devices":
				if r.Header.Get("Authorization") != "Bearer short-lived" {
					t.Fatalf("unexpected inventory authorization")
				}
				_, _ = w.Write([]byte(`{"devices":[{"id":"d1","name":"db","hostname":"db","addresses":["198.51.100.1"],"os":"linux","clientVersion":"1.2","lastSeen":"now","authorized":true}]}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()
		c := Connection{ID: "c2", Label: "ts", Provider: Tailscale, Config: json.RawMessage(`{"tailnet":"example.com","client_id":"client-1"}`), SyncIntervalSeconds: 300}
		items, err := inventory(context.Background(), c, []byte("ts-secret"), srv.Client(), providerURLs{tailscale: srv.URL, tailscaleToken: srv.URL + "/oauth/token"})
		if err != nil || len(items) != 1 || items[0].Address != "198.51.100.1" || items[0].State != "active" {
			t.Fatalf("items=%+v err=%v", items, err)
		}
	})

	t.Run("custom inventory discards provider supplied details", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer custom-secret" {
				t.Fatalf("missing bearer authorization")
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"x","kind":"vm","name":"node","details":{"password":"must not persist"}}]}`))
		}))
		defer srv.Close()
		c := Connection{ID: "c1", Label: "test", Provider: Custom, Config: json.RawMessage(`{"inventory_url":"https://127.0.0.1/inventory"}`), SyncIntervalSeconds: 300}
		// Inject the test server through the configured URL after local validation.
		cfg := Config{InventoryURL: srv.URL}
		items, err := customInventory(context.Background(), srv.Client(), c, cfg, "custom-secret")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || strings.Contains(string(items[0].Details), "password") {
			t.Fatalf("provider supplied details should not be persisted: %+v", items)
		}
	})

	t.Run("redirects are rejected", func(t *testing.T) {
		destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"items":[]}`)) }))
		defer destination.Close()
		redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", destination.URL)
			w.WriteHeader(http.StatusFound)
		}))
		defer redirect.Close()
		c := Connection{ID: "c1", Label: "test", Provider: Custom, SyncIntervalSeconds: 300}
		hc := redirect.Client()
		hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		_, err := customInventory(context.Background(), hc, c, Config{InventoryURL: redirect.URL}, "")
		if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
			t.Fatalf("expected redirect rejection, got %v", err)
		}
	})
}

func TestNetworkConnectionValidationRejectsUnsafeCustomURLs(t *testing.T) {
	for _, raw := range []string{"http://example.com/inventory", "https://user:pass@example.com/inventory", "https://example.com/inventory?token=x", "https://example.com/inventory#fragment"} {
		c := Connection{Label: "custom", Provider: Custom, Config: json.RawMessage(`{"inventory_url":"` + raw + `"}`), SyncIntervalSeconds: 300}
		if err := c.Validate(); err == nil {
			t.Errorf("accepted unsafe URL %q", raw)
		}
	}
}

func TestNetworkConnectionValidationRejectsUnknownFieldsAndBadIntervals(t *testing.T) {
	for _, test := range []struct {
		config   string
		interval int
	}{
		{`{"account_id":"account-1","account":"typo"}`, 300},
		{`{"account_id":"account-1"}`, 59},
		{`{"account_id":"account-1"}`, 86401},
	} {
		c := Connection{Label: "cf", Provider: CloudflareTunnel, Config: json.RawMessage(test.config), SyncIntervalSeconds: test.interval}
		if err := c.Validate(); err == nil {
			t.Errorf("accepted config=%s interval=%d", test.config, test.interval)
		}
	}
}
