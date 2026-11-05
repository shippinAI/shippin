package incus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/hv"
)

const testOCIReference = "registry.example.net/team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseOCIApplicationReferenceRequiresQualifiedImmutableImage(t *testing.T) {
	got, err := parseOCIApplicationReference(testOCIReference)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server != "https://registry.example.net" || got.Alias != "team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("parsed reference = %+v", got)
	}
	for _, reference := range []string{
		"postgres:17",
		"registry.example.net/team/postgres:17",
		"registry.example.net/team/postgres@sha256:short",
		"registry.example.net/team/../postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"registry.example.net/team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n",
		"registry.example.net/team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		t.Run(reference, func(t *testing.T) {
			if _, err := parseOCIApplicationReference(reference); err == nil {
				t.Fatal("accepted mutable, malformed or unsafe image reference")
			}
		})
	}
}

func TestCreateOCIApplicationPreflightsVersionWaitsAndReadsBack(t *testing.T) {
	status := "Stopped"
	created := false
	var create map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-1":
			if !created {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
				"name": "app-1", "status": status, "type": "container",
				"config": map[string]string{"volatile.container.oci": "true"},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances":
			if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
				t.Errorf("decode create request: %v", err)
			}
			created = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"operation-1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/operation-1/wait":
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/app-1/state":
			status = "Running"
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	got, err := c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2, Start: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-1" || got.Status != "Running" || got.Kind != hv.KindCT {
		t.Fatalf("read-back = %+v", got)
	}
	source, ok := create["source"].(map[string]any)
	if !ok || source["type"] != "image" || source["mode"] != "pull" || source["protocol"] != "oci" ||
		source["server"] != "https://registry.example.net" || source["alias"] != "team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("OCI source = %#v", create["source"])
	}
	config := create["config"].(map[string]any)
	if config["limits.cpu"] != "2" || config["limits.memory"] != "1024MiB" {
		t.Fatalf("resource configuration = %#v", config)
	}
}

func TestCreateOCIApplicationRejectsOldIncusBeforeMutation(t *testing.T) {
	creates := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1.0" {
			_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.2.9"}}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/1.0/instances" {
			creates++
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	_, err := c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2,
	})
	if err == nil || !strings.Contains(err.Error(), "7.3.0 or newer") {
		t.Fatalf("old server error = %v", err)
	}
	if creates != 0 {
		t.Fatalf("old Incus server received %d mutations", creates)
	}
}

func TestCreateOCIApplicationRejectsInvalidResourcesBeforeProviderCall(t *testing.T) {
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	_, err := c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", ImageReference: testOCIReference, MemoryMiB: 64, Cores: 2,
	})
	if err == nil || requests != 0 {
		t.Fatalf("validation error=%v provider requests=%d", err, requests)
	}
}

func TestOCIApplicationReadbackRequiresOCIContainerAndRequestedState(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata string
		started  bool
		wantErr  bool
	}{
		{name: "non oci", metadata: `{"name":"app-1","status":"Running","type":"container","config":{"volatile.container.oci":"false"}}`, started: true, wantErr: true},
		{name: "not running", metadata: `{"name":"app-1","status":"Stopped","type":"container","config":{"volatile.container.oci":"true"}}`, started: true, wantErr: true},
		{name: "valid stopped", metadata: `{"name":"app-1","status":"Stopped","type":"container","config":{"volatile.container.oci":"true"}}`, wantErr: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/1.0/instances/app-1" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte(`{"metadata":` + test.metadata + `}`))
			}))
			defer srv.Close()
			c := clientFor(t, srv)
			_, err := c.applicationReadback(context.Background(), "app-1", test.started)
			if (err != nil) != test.wantErr {
				t.Fatalf("read-back error = %v", err)
			}
		})
	}
}
