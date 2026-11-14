// Package runnerbridge connects the existing panel to the separate runner session
// gateway. Runner signing keys and authorization logic remain owned by that service.
package runnerbridge

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Proxy struct {
	origin string
	client *http.Client
}

// New leaves the integration explicitly unavailable when configuration is absent.
// A partial or invalid configuration fails activation, not TLS verification.
func New(origin, caFile string) (*Proxy, error) {
	if origin == "" && caFile == "" {
		return &Proxy{}, nil
	}
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != origin || !filepath.IsAbs(caFile) {
		return nil, errors.New("invalid runner gateway configuration")
	}
	st, e := os.Lstat(caFile)
	if e != nil || !st.Mode().IsRegular() || st.Size() < 1 || st.Size() > 65536 {
		return nil, errors.New("runner gateway trust unavailable")
	}
	raw, e := os.ReadFile(caFile)
	if e != nil {
		return nil, errors.New("runner gateway trust unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(raw) {
		return nil, errors.New("invalid runner gateway trust")
	}
	c := &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 65536, IdleConnTimeout: 30 * time.Second, MaxIdleConnsPerHost: 4}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Proxy{origin: origin, client: c}, nil
}
func (p *Proxy) Register(mux *http.ServeMux) {
	mux.Handle("/v1/runner/projects", p)
	mux.Handle("/v1/projects/", p)
}
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if p.client == nil {
		problem(w, 503, "Runner service is not configured.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		problem(w, 405, "Runner action is not supported.")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		problem(w, 400, "Runner request is invalid.")
		return
	}
	var cookie *http.Cookie
	for _, c := range r.Cookies() {
		if c.Name == "shippin_session" {
			if cookie != nil {
				problem(w, 401, "Sign in to use runners.")
				return
			}
			cookie = c
		}
	}
	if cookie == nil || len(cookie.Value) < 16 || len(cookie.Value) > 512 {
		problem(w, 401, "Sign in to use runners.")
		return
	}
	var body io.Reader
	if r.Method == http.MethodPost {
		body = http.MaxBytesReader(w, r.Body, 4096)
	}
	req, e := http.NewRequestWithContext(r.Context(), r.Method, p.origin+r.URL.Path, body)
	if e != nil {
		problem(w, 503, "Runner service is unavailable.")
		return
	}
	req.AddCookie(&http.Cookie{Name: "shippin_session", Value: cookie.Value})
	for _, name := range []string{"Origin", "Content-Type", "Idempotency-Key"} {
		for _, v := range r.Header.Values(name) {
			req.Header.Add(name, v)
		}
	}
	res, e := p.client.Do(req)
	if e != nil {
		problem(w, 503, "Runner service is unavailable.")
		return
	}
	defer res.Body.Close()
	data, e := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if e != nil || len(data) > 1<<20 {
		problem(w, 502, "Runner response is unavailable.")
		return
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		switch res.StatusCode {
		case 400, 401, 403, 404, 409, 503:
			problem(w, res.StatusCode, "Runner request could not be completed.")
		default:
			problem(w, 502, "Runner response is unavailable.")
		}
		return
	}
	// Only gateway-sanitized JSON/text bodies, never private headers or cookies.
	ct := res.Header.Get("Content-Type")
	if ct != "application/json" && ct != "text/plain; charset=utf-8" && ct != "text/plain" {
		problem(w, 502, "Runner response is unavailable.")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(data)
}
func problem(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Title  string `json:"title"`
		Status int    `json:"status"`
	}{message, status})
}
