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
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Alias    string `json:"alias,omitempty"`
	Kind     string `json:"kind"`
	Instance string `json:"instance"`
	Healthy  bool   `json:"healthy"`
	// Configured is false when nobody has said where this service is. That is not the same as down.
	Configured     bool             `json:"configured"`
	LatencyMs      *int64           `json:"latency_ms,omitempty"`
	Detail         string           `json:"detail"`
	TimeoutSeconds int              `json:"timeout_seconds"`
	State          string           `json:"state"`
	ObservedAt     *time.Time       `json:"observed_at,omitempty"`
	Resource       *serviceResource `json:"resource,omitempty"`
}

type serviceResource struct {
	ServerID      string            `json:"server_id"`
	GuestID       string            `json:"guest_id"`
	Server        string            `json:"server,omitempty"`
	Name          string            `json:"name,omitempty"`
	Kind          string            `json:"kind,omitempty"`
	Status        string            `json:"status,omitempty"`
	Observed      bool              `json:"observed"`
	Detail        string            `json:"detail"`
	CheckedAt     time.Time         `json:"checked_at"`
	Metrics       *hv.GuestMetrics  `json:"metrics,omitempty"`
	Addresses     []hv.GuestAddress `json:"addresses,omitempty"`
	NetworkDetail string            `json:"network_detail,omitempty"`
}

type probeConcurrencyGate struct {
	mu      sync.Mutex
	active  int
	limit   int
	changed chan struct{}
}

func newProbeConcurrencyGate(limit int) *probeConcurrencyGate {
	if limit < 1 || limit > config.MaxProbeConcurrency {
		limit = config.DefaultProbeConcurrency
	}
	return &probeConcurrencyGate{limit: limit, changed: make(chan struct{})}
}

func (g *probeConcurrencyGate) setLimit(limit int) {
	if limit < 1 || limit > config.MaxProbeConcurrency {
		limit = config.DefaultProbeConcurrency
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.limit == limit {
		return
	}
	g.limit = limit
	close(g.changed)
	g.changed = make(chan struct{})
}

func (g *probeConcurrencyGate) acquire(ctx context.Context) bool {
	for {
		g.mu.Lock()
		if g.active < g.limit {
			g.active++
			g.mu.Unlock()
			return true
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-changed:
		}
	}
}

func (g *probeConcurrencyGate) release() {
	g.mu.Lock()
	g.active--
	close(g.changed)
	g.changed = make(chan struct{})
	g.mu.Unlock()
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
	if observer, ok := driver.(hv.GuestNetworkObserver); ok {
		addresses, networkErr := observer.GuestNetwork(ctx, guest)
		if networkErr == nil {
			out.Addresses = addresses
			out.NetworkDetail = "provider-reported addresses; control-plane reachability not verified"
		} else {
			out.NetworkDetail = "provider network observation unavailable"
		}
	} else {
		out.NetworkDetail = "provider does not expose guest addresses"
	}
	if guest.Metrics == nil {
		out.Detail = "provider did not report resource metrics"
	} else {
		out.Detail = "observed"
	}
	return out
}

func (a *API) platformProbeConcurrency(ctx context.Context) int {
	a.probeLimitMu.Lock()
	defer a.probeLimitMu.Unlock()
	if a.probeGate == nil {
		a.probeGate = newProbeConcurrencyGate(config.DefaultProbeConcurrency)
	}
	limit := config.DefaultProbeConcurrency
	var configured any
	if a.db != nil {
		configured = a.platformValue(ctx, "services.probe_concurrency")
	}
	switch value := configured.(type) {
	case int:
		if value >= 1 && value <= config.MaxProbeConcurrency {
			limit = value
		}
	case float64: // PostgreSQL JSON values are decoded as float64.
		if value >= 1 && value <= config.MaxProbeConcurrency && value == float64(int(value)) {
			limit = int(value)
		}
	}
	a.probeGate.setLimit(limit)
	return limit
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
				if !a.probeGate.acquire(ctx) {
					results[i] = probeResult{Name: definitions[i].Name, Kind: definitions[i].Kind,
						TimeoutSeconds: definitions[i].TimeoutSeconds, Detail: "check cancelled"}
					continue
				}
				results[i] = a.serviceProbe(ctx, definitions[i])
				a.probeGate.release()
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
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: cfg},
		// Health checks are operator-configured network requests. Do not follow
		// redirects: a configured service must not be able to redirect a probe
		// into a different network destination (for example, a metadata endpoint).
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
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
	// A 3xx is not a successful health response. Redirects are intentionally not
	// followed to keep the configured origin as the only network destination.
	res.Healthy, res.Detail = r.StatusCode >= http.StatusOK && r.StatusCode < http.StatusMultipleChoices,
		http.StatusText(r.StatusCode)
	return res
}

// services returns persisted probe observations; reading the dashboard does no network I/O.
func (a *API) services(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	a.servicesFor(w, r, c)
}

func (a *API) servicesFor(w http.ResponseWriter, r *http.Request, c *caller) {
	if c.op == "" {
		auth.Problem(w, http.StatusForbidden, "Platform services need an operator role.")
		return
	}
	definitions, err := a.db.PlatformServices(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Platform health registry is unavailable.")
		return
	}
	observations, err := a.db.LatestPlatformServiceObservations(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Platform health observations are unavailable.")
		return
	}
	probes := make([]probeResult, 0, len(definitions))
	for _, s := range definitions {
		if !s.Enabled {
			continue
		}
		result := probeResult{ID: s.ID, Name: s.Name, Alias: s.Alias, Kind: s.Kind, Instance: s.Instance, TimeoutSeconds: s.TimeoutSeconds, State: "unknown", Detail: "awaiting first observation"}
		var observation struct {
			Revision   int64           `json:"config_revision"`
			ObservedAt time.Time       `json:"observed_at"`
			Configured bool            `json:"configured"`
			Healthy    bool            `json:"healthy"`
			LatencyMs  *int64          `json:"latency_ms"`
			Detail     string          `json:"detail"`
			Resource   json.RawMessage `json:"resource"`
		}
		if raw := observations[s.ID]; len(raw) > 0 && json.Unmarshal(raw, &observation) == nil && observation.Revision == s.Revision {
			result.Configured, result.Healthy, result.LatencyMs, result.Detail = observation.Configured, observation.Healthy, observation.LatencyMs, observation.Detail
			result.ObservedAt = &observation.ObservedAt
			if time.Since(observation.ObservedAt) > time.Duration(s.IntervalSeconds*2)*time.Second {
				result.State = "stale"
			} else if observation.Healthy {
				result.State = "operational"
			} else {
				result.State = "down"
			}
			if len(observation.Resource) > 0 && string(observation.Resource) != "null" {
				_ = json.Unmarshal(observation.Resource, &result.Resource)
			}
		}
		probes = append(probes, result)
	}
	// Keep the existing live Incus instance inventory alongside persisted service
	// observations. The registry removes probe I/O from dashboard reads; it does
	// not replace hostd inventory or its explicit unavailable state.
	insts, ierr := a.instances(r.Context())
	out := map[string]any{"project": a.cfg.Incus.Project, "host": a.cfg.Incus.HostLabel,
		"probes": probes, "instances": insts, "checked_at": time.Now().UTC()}
	if ierr != nil {
		out["instances_error"] = "Incus read-only gateway unavailable"
	}
	writeJSON(w, out)
}
