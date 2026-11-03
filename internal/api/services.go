package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

type probeResult struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Instance string `json:"instance"`
	Healthy  bool   `json:"healthy"`
	// Configured is false when nobody has said where this service is. That is not the same as down.
	Configured bool   `json:"configured"`
	LatencyMs  int64  `json:"latency_ms"`
	Detail     string `json:"detail"`
}

func (a *API) platformProbes(ctx context.Context) []config.Probe {
	value := a.platformValue(ctx, "services.probes")
	raw, err := json.Marshal(value)
	var probes []config.Probe
	if err != nil || json.Unmarshal(raw, &probes) != nil {
		return a.cfg.Probes
	}
	return probes
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
	// A named service without a target is configuration, not a successful probe.
	// Keep the health page useful during first-run without panicking on an empty URL.
	if (p.TCP == "") == (p.URL == "") {
		res.Detail = "not configured"
		return res
	}
	res.Configured = true
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		res.Detail = "invalid probe URL"
		return res
	}
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
	definitions := a.platformProbes(r.Context())
	probes := make([]probeResult, len(definitions))
	var wg sync.WaitGroup
	for i, p := range definitions {
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
