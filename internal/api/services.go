package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"sync"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

type probeResult struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Instance  string `json:"instance"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int64  `json:"latency_ms"`
	Detail    string `json:"detail"`
}

// probeClient verifies TLS against the system trust store. serverName sets SNI when the dial host differs.
func probeClient(serverName string) *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	cfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if serverName != "" {
		cfg.ServerName = serverName
	}
	return &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg}}
}

func probe(ctx context.Context, p config.Probe) (res probeResult) {
	res = probeResult{Name: p.Name, Kind: p.Kind, Instance: p.Instance}
	t0 := time.Now()
	defer func() { res.LatencyMs = time.Since(t0).Milliseconds() }()
	if p.TCP != "" {
		c, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", p.TCP)
		if err == nil {
			c.Close()
			res.Healthy, res.Detail = true, "accepting connections"
		} else {
			res.Detail = "unreachable"
		}
		return res
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if p.Host != "" {
		req.Host = p.Host
	}
	r, err := probeClient(p.Host).Do(req)
	if err != nil {
		res.Detail = "unreachable"
		return res
	}
	r.Body.Close()
	res.Healthy, res.Detail = r.StatusCode < 400, http.StatusText(r.StatusCode)
	return res
}

// services returns live probes plus Incus instance state (operator only).
func (a *API) services(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	if c.op == "" {
		auth.Problem(w, http.StatusForbidden, "Platform services need an operator role.")
		return
	}
	probes := make([]probeResult, len(a.cfg.Probes))
	var wg sync.WaitGroup
	for i, p := range a.cfg.Probes {
		wg.Add(1)
		go func() { defer wg.Done(); probes[i] = probe(r.Context(), p) }()
	}
	wg.Wait()
	insts, ierr := a.instances(r.Context())
	out := map[string]any{"project": a.cfg.Incus.Project, "host": a.cfg.Incus.HostLabel,
		"probes": probes, "instances": insts, "checked_at": time.Now().UTC()}
	if ierr != nil {
		out["instances_error"] = "Incus read-only gateway unavailable"
	}
	writeJSON(w, out)
}
