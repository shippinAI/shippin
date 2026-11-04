package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/servers"
)

func TestProbeDoesNotClaimMissingTargetIsHealthy(t *testing.T) {
	got := probe(context.Background(), config.Probe{Name: "Nomen", Kind: "nomen", Instance: "identity"})
	if got.Healthy || got.Detail != "not configured" || got.LatencyMs != nil {
		t.Fatalf("missing endpoint reported as healthy: %#v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil || strings.Contains(string(raw), `"latency_ms"`) {
		t.Fatalf("unconfigured probe must omit unmeasured latency: json=%s err=%v", raw, err)
	}
}

func TestProbeTellsNotSetUpFromDown(t *testing.T) {
	if got := probe(context.Background(), config.Probe{Name: "Nomen", Kind: "nomen"}); got.Configured || got.Healthy {
		t.Fatalf("no target must read as not set up: %+v", got)
	}
	if got := probe(context.Background(), config.Probe{Name: "Nomen", Kind: "nomen", TCP: "127.0.0.1:1"}); !got.Configured || got.Healthy || got.LatencyMs == nil {
		t.Fatalf("a target that does not answer must read as down: %+v", got)
	}
}

func TestProbeRejectsInvalidURLAndChecksConfiguredEndpoint(t *testing.T) {
	invalid := probe(context.Background(), config.Probe{URL: "://bad"})
	if invalid.Healthy || invalid.Detail != "invalid probe URL" {
		t.Fatalf("invalid URL result: %#v", invalid)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	got := probe(context.Background(), config.Probe{URL: server.URL})
	if !got.Healthy || got.Detail != "No Content" {
		t.Fatalf("configured endpoint not probed: %#v", got)
	}
}

func TestProbeHonorsConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	started := time.Now()
	got := probe(context.Background(), config.Probe{URL: server.URL, TimeoutSeconds: 1})
	if got.Healthy || got.TimeoutSeconds != 1 || time.Since(started) < time.Second {
		t.Fatalf("configured timeout was not applied: result=%#v elapsed=%s", got, time.Since(started))
	}
}

type serviceGuestDriver struct {
	hv.Driver
	guest hv.Guest
	err   error
}

func (d serviceGuestDriver) Guest(_ context.Context, id string) (hv.Guest, error) {
	if id != d.guest.ID {
		return hv.Guest{}, hv.ErrGuestNotFound
	}
	return d.guest, d.err
}

func TestServiceProbeJoinsProviderReportedResources(t *testing.T) {
	const serverID = "019a0000-0000-7000-8000-0000000000a1"
	api := &API{
		hvServers: &memHV{items: []servers.Endpoint{{ID: serverID, Kind: "proxmox", Label: "Build node"}}},
		openHV: func(servers.Endpoint) (hv.Driver, error) {
			return serviceGuestDriver{guest: hv.Guest{ID: "101", Name: "postgres", Kind: hv.KindVM, Status: "running", Metrics: &hv.GuestMetrics{
				CPU: .25, CPUCount: 4, Memory: 512 << 20, MaxMemory: 2 << 30, Disk: 8 << 30, MaxDisk: 40 << 30,
			}}}, nil
		},
	}
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer health.Close()
	got := api.serviceProbe(context.Background(), config.Probe{Name: "Database", Kind: "database", URL: health.URL, ServerID: serverID, GuestID: "101"})
	if !got.Healthy || got.Resource == nil || !got.Resource.Observed || got.Resource.Server != "Build node" || got.Resource.Status != "running" {
		t.Fatalf("provider resource was not joined: %+v", got)
	}
	if got.Resource.Metrics == nil || got.Resource.Metrics.CPU != .25 || got.Resource.Metrics.Memory != 512<<20 || got.Resource.Metrics.MaxDisk != 40<<30 {
		t.Fatalf("provider metrics were not preserved: %+v", got.Resource.Metrics)
	}
}

func TestServiceProbeKeepsHealthSeparateFromMissingProviderResource(t *testing.T) {
	const serverID = "019a0000-0000-7000-8000-0000000000a1"
	api := &API{
		hvServers: &memHV{items: []servers.Endpoint{{ID: serverID, Kind: "proxmox", Label: "Build node"}}},
		openHV: func(servers.Endpoint) (hv.Driver, error) {
			return serviceGuestDriver{guest: hv.Guest{ID: "101"}, err: errors.New("provider unavailable")}, nil
		},
	}
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer health.Close()
	got := api.serviceProbe(context.Background(), config.Probe{Name: "Database", Kind: "database", URL: health.URL, ServerID: serverID, GuestID: "101"})
	if !got.Healthy || got.Resource == nil || got.Resource.Observed || got.Resource.Detail != "resource unavailable" {
		t.Fatalf("provider failure must not overwrite independent service health: %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil || strings.Contains(string(raw), "provider unavailable") || strings.Contains(string(raw), health.URL) {
		t.Fatalf("resource errors must not expose provider internals: %s err=%v", raw, err)
	}
}

func TestProbeProviderResourceReferenceRequiresServerAndGuest(t *testing.T) {
	base := config.Probe{Name: "Database", Kind: "database", URL: "https://database.example.test/health"}
	for _, p := range []config.Probe{{Name: base.Name, Kind: base.Kind, URL: base.URL, ServerID: "server"}, {Name: base.Name, Kind: base.Kind, URL: base.URL, GuestID: "101"}} {
		if err := validateProbes([]config.Probe{p}); err == nil {
			t.Fatalf("incomplete provider reference must be rejected: %+v", p)
		}
	}
	base.ServerID, base.GuestID = "019a0000-0000-7000-8000-0000000000a1", "101"
	if err := validateProbes([]config.Probe{base}); err != nil {
		t.Fatalf("paired provider reference should be accepted: %v", err)
	}
	base.ServerID = "installation-server-name"
	if err := validateProbes([]config.Probe{base}); err == nil {
		t.Fatal("provider reference must use the stable system UUID")
	}
	base.ServerID, base.GuestID = "019a0000-0000-7000-8000-0000000000a1", "guest\tname"
	if err := validateProbes([]config.Probe{base}); err == nil {
		t.Fatal("provider guest references must reject control characters")
	}
}

func TestServiceProbesRespectConfiguredConcurrency(t *testing.T) {
	var active, maximum atomic.Int32
	release := make(chan struct{})
	ready := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		if current == 2 {
			once.Do(func() { close(ready) })
		}
		<-release
		active.Add(-1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	definitions := make([]config.Probe, 8)
	for i := range definitions {
		definitions[i] = config.Probe{Name: fmt.Sprintf("service-%d", i), Kind: "database", URL: server.URL}
	}
	api := &API{}
	done := make(chan []probeResult, 1)
	go func() { done <- api.serviceProbes(context.Background(), definitions, 2) }()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("the configured worker limit did not start two probes")
	}
	close(release)
	results := <-done
	if len(results) != len(definitions) || maximum.Load() != 2 {
		t.Fatalf("probe concurrency exceeded or failed to fill the configured limit: results=%d max=%d", len(results), maximum.Load())
	}
}

func TestPlatformProbeConcurrencySettingHasSafeBounds(t *testing.T) {
	a := &API{cfg: &config.Config{ProbeConcurrency: config.DefaultProbeConcurrency}}
	var validate func(any) error
	for _, setting := range a.platformKeys() {
		if setting.Key == "services.probe_concurrency" {
			validate = setting.valid
			break
		}
	}
	if validate == nil {
		t.Fatal("concurrency setting is missing")
	}
	for _, value := range []any{float64(0), float64(1.5), float64(config.MaxProbeConcurrency + 1), "4"} {
		if err := validate(value); err == nil {
			t.Fatalf("invalid concurrency value accepted: %#v", value)
		}
	}
	for _, value := range []any{float64(1), float64(config.MaxProbeConcurrency)} {
		if err := validate(value); err != nil {
			t.Fatalf("valid boundary rejected (%v): %v", value, err)
		}
	}
}
