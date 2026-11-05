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

func TestProbeDoesNotFollowRedirectsToOtherDestinations(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer origin.Close()

	got := probe(context.Background(), config.Probe{URL: origin.URL})
	if !got.Healthy || !got.Configured || redirected.Load() {
		t.Fatalf("probe followed redirect or misreported configured origin: result=%+v redirected=%v", got, redirected.Load())
	}
}

type serviceGuestDriver struct {
	hv.Driver
	guest      hv.Guest
	err        error
	networkErr error
}

func (d serviceGuestDriver) GuestNetwork(context.Context, hv.Guest) ([]hv.GuestAddress, error) {
	if d.networkErr != nil {
		return nil, d.networkErr
	}
	return []hv.GuestAddress{{Interface: "eth0", Address: "192.0.2.22", Family: "ipv4", Scope: "global"}}, nil
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
	if len(got.Resource.Addresses) != 1 || got.Resource.Addresses[0].Address != "192.0.2.22" || !strings.Contains(got.Resource.NetworkDetail, "reachability not verified") {
		t.Fatalf("provider address must be surfaced without claiming control-plane reachability: %+v", got.Resource)
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

func TestServiceProbeKeepsHealthSeparateFromUnavailableGuestNetwork(t *testing.T) {
	const serverID = "019a0000-0000-7000-8000-0000000000a1"
	api := &API{
		hvServers: &memHV{items: []servers.Endpoint{{ID: serverID, Kind: "proxmox", Label: "Build node"}}},
		openHV: func(servers.Endpoint) (hv.Driver, error) {
			return serviceGuestDriver{guest: hv.Guest{ID: "101", Name: "postgres", Kind: hv.KindVM, Status: "running"}, networkErr: errors.New("guest agent unavailable")}, nil
		},
	}
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer health.Close()
	got := api.serviceProbe(context.Background(), config.Probe{Name: "Database", Kind: "database", URL: health.URL, ServerID: serverID, GuestID: "101"})
	if !got.Healthy || got.Resource == nil || !got.Resource.Observed || len(got.Resource.Addresses) != 0 || got.Resource.NetworkDetail != "provider network observation unavailable" {
		t.Fatalf("missing guest-agent network data must not overwrite independent health or expose provider errors: %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil || strings.Contains(string(raw), "guest agent unavailable") {
		t.Fatalf("provider network error escaped into API response: %s err=%v", raw, err)
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
	api := &API{probeGate: newProbeConcurrencyGate(2)}
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

func TestServiceProbeConcurrencyIsSharedAcrossRequests(t *testing.T) {
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
	definitions := func(prefix string) []config.Probe {
		probes := make([]config.Probe, 4)
		for i := range probes {
			probes[i] = config.Probe{Name: fmt.Sprintf("%s-%d", prefix, i), Kind: "database", URL: server.URL}
		}
		return probes
	}
	api := &API{probeGate: newProbeConcurrencyGate(2)}
	var workers sync.WaitGroup
	workers.Add(2)
	for _, prefix := range []string{"first", "second"} {
		go func(prefix string) {
			defer workers.Done()
			results := api.serviceProbes(context.Background(), definitions(prefix), 2)
			if len(results) != 4 {
				t.Errorf("request %s got %d observations", prefix, len(results))
			}
		}(prefix)
	}
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("shared gate did not admit two concurrent probes")
	}
	close(release)
	workers.Wait()
	if maximum.Load() != 2 {
		t.Fatalf("concurrent requests exceeded the shared configured limit: max=%d", maximum.Load())
	}
}
