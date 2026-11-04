package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/config"
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
