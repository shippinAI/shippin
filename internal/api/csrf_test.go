package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := SameOrigin(ok)
	cases := []struct {
		method, origin, site string
		want                 int
	}{
		{"POST", "https://console.shippin.cloud", "same-origin", 204},
		{"POST", "https://evil--site.ws.shippin.cloud", "same-site", 403},
		{"PUT", "https://x.ws.shippin.cloud", "", 403},
		{"DELETE", "", "cross-site", 403},
		{"POST", "null", "", 403},
		{"POST", "", "", 204}, // non-browser client (no cookie path)
		{"GET", "https://evil.example", "cross-site", 204},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "https://console.shippin.cloud/v1/tenants/t/workspaces/w/site", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.site != "" {
			r.Header.Set("Sec-Fetch-Site", c.site)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s origin=%q site=%q: got %d want %d", c.method, c.origin, c.site, w.Code, c.want)
		}
	}
}

func TestSameOriginNonDefaultPort(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := SameOrigin(ok)
	for _, c := range []struct {
		origin string
		want   int
	}{
		{"https://panel.example.invalid:8443", 204},
		{"https://panel.example.invalid", 403},
		{"https://panel.example.invalid:9443", 403},
	} {
		r := httptest.NewRequest("POST", "https://panel.example.invalid:8443/v1/setup", nil)
		r.Header.Set("Origin", c.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("origin=%q: got %d want %d", c.origin, w.Code, c.want)
		}
	}
}
