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
	"shippin.cloud/console/internal/hv"
)

type probeResult struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Instance string `json:"instance"`
	Healthy  bool   `json:"healthy"`
	// Configured is false when nobody has said where this service is. That is not the same as down.
	Configured     bool             `json:"configured"`
	LatencyMs      *int64           `json:"latency_ms,omitempty"`
	Detail         string           `json:"detail"`
	TimeoutSeconds int              `json:"timeout_seconds"`
	Resource       *serviceResource `json:"resource,omitempty"`
}

type serviceResource struct {
	ServerID  string           `json:"server_id"`
	GuestID   string           `json:"guest_id"`
	Server    string           `json:"server,omitempty"`
	Name      string           `json:"name,omitempty"`
	Kind      string           `json:"kind,omitempty"`
	Status    string           `json:"status,omitempty"`
	Observed  bool             `json:"observed"`
	Detail    string           `json:"detail"`
	CheckedAt time.Time        `json:"checked_at"`
	Metrics   *hv.GuestMetrics `json:"metrics,omitempty"`
}

func (a *API) serviceProbe(ctx context.Context, p config.Probe) probeResult {
	res := probe(ctx, p)
	if p.ServerID == "" {
		return res
	}
	res.Resource = a.serviceResource(ctx, p)
	return res
}

func (a *API) serviceResource(ctx context.Context, p config.Probe) *serviceResource {
	out := &serviceResource{ServerID: p.ServerID, GuestID: p.GuestID, Detail: "resource unavailable", CheckedAt: time.Now().UTC()}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	ep, err := a.hvDir().Endpoint(ctx, p.ServerID)
	if err != nil {
		out.Detail = "system unavailable"
		return out
	}
	out.Server = ep.Label
	driver, err := a.openDriver(ep)
	if err != nil {
		return out
	}
	guest, err := driver.Guest(ctx, p.GuestID)
	if err != nil {
		return out
	}
	out.Name, out.Kind, out.Status, out.Metrics, out.Observed = guest.Name, guest.Kind, guest.Status, guest.Metrics, true
	if guest.Metrics == nil {
		out.Detail = "provider did not report resource metrics"
	} else {
		out.Detail = "observed"
	}
	return out
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

func (a *API) platformProbeConcurrency(ctx context.Context) int {
	switch value := a.platformValue(ctx, "services.probe_concurrency").(type) {
	case int:
		return value
	case float64: // PostgreSQL JSON values are decoded as float64.
		if value >= 1 && value <= config.MaxProbeConcurrency && value == float64(int(value)) {
			return int(value)
		}
	}
	return config.DefaultProbeConcurrency
}

func (a *API) serviceProbes(ctx context.Context, definitions []config.Probe, limit int) []probeResult {
	results := make([]probeResult, len(definitions))
	if len(definitions) == 0 {
		return results
	}
	if limit < 1 || limit > config.MaxProbeConcurrency {
		limit = config.DefaultProbeConcurrency
	}
	if limit > len(definitions) {
		limit = len(definitions)
	}
	indices := make(chan int)
	var workers sync.WaitGroup
	workers.Add(limit)
	for i := 0; i < limit; i++ {
		go func() {
			defer workers.Done()
			for i := range indices {
				results[i] = a.serviceProbe(ctx, definitions[i])
			}
		}()
	}
	for i := range definitions {
		indices <- i
	}
	close(indices)
	workers.Wait()
	return results
}

// probeClient verifies TLS against the system trust store. serverName sets SNI when the dial host differs.
func probeClient(serverName string, timeout time.Duration) *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	cfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if serverName != "" {
		cfg.ServerName = serverName
	}
	return &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: cfg}}
}

func probe(ctx context.Context, p config.Probe) (res probeResult) {
	res = probeResult{Name: p.Name, Kind: p.Kind, Instance: p.Instance, TimeoutSeconds: p.TimeoutSeconds}
	timeout := 3 * time.Second
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	} else {
		res.TimeoutSeconds = 3
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// A named service without a target is configuration, not a successful probe.
	// Keep the health page useful during first-run without panicking on an empty URL.
	if (p.TCP == "") == (p.URL == "") {
		res.Detail = "not configured"
		return res
	}
	res.Configured = true
	t0 := time.Now()
	defer func() {
		latency := time.Since(t0).Milliseconds()
		res.LatencyMs = &latency
	}()
	if p.TCP != "" {
		c, err := (&net.Dialer{Timeout: timeout}).DialContext(probeCtx, "tcp", p.TCP)
		if err == nil {
			c.Close()
			res.Healthy, res.Detail = true, "accepting connections"
		} else {
			res.Detail = "unreachable"
		}
		return res
	}
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, p.URL, nil)
	if err != nil {
		res.Detail = "invalid probe URL"
		return res
	}
	if p.Host != "" {
		req.Host = p.Host
	}
	r, err := probeClient(p.Host, timeout).Do(req)
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
	probes := a.serviceProbes(r.Context(), definitions, a.platformProbeConcurrency(r.Context()))
	insts, ierr := a.instances(r.Context())
	out := map[string]any{"project": a.cfg.Incus.Project, "host": a.cfg.Incus.HostLabel,
		"probes": probes, "instances": insts, "checked_at": time.Now().UTC()}
	if ierr != nil {
		out["instances_error"] = "Incus read-only gateway unavailable"
	}
	writeJSON(w, out)
}
