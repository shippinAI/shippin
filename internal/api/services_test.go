package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/config"
)

func TestProbeDoesNotClaimMissingTargetIsHealthy(t *testing.T) {
	got := probe(context.Background(), config.Probe{Name: "Nomen", Kind: "nomen", Instance: "identity"})
	if got.Healthy || got.Detail != "not configured" {
		t.Fatalf("missing endpoint reported as healthy: %#v", got)
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
