package host_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"warden/internal/host"
)

// fixtureRoot points at the testdata/ directory next to this test file.
// Go's `go test` runner sets cwd to the package directory.
const fixtureRoot = "testdata"

func TestRegistry_Load_FixturesParse(t *testing.T) {
	r := host.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := r.Count(), 2; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
}

func TestRegistry_Get_KnownHost(t *testing.T) {
	r := host.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	h := r.Get("server1")
	if h == nil {
		t.Fatal("Get(server1) = nil; want non-nil")
	}
	if h.Name != "server1" {
		t.Errorf("Name = %q, want server1", h.Name)
	}
	if h.Kind != host.KindProxmox {
		t.Errorf("Kind = %q, want %q", h.Kind, host.KindProxmox)
	}
	if h.OS != "Debian 12" {
		t.Errorf("OS = %q, want %q", h.OS, "Debian 12")
	}
	if !h.Kind.IsKnown() {
		t.Errorf("Kind.IsKnown() = false; want true")
	}
	if got := len(h.Addresses); got != 3 {
		t.Errorf("len(Addresses) = %d, want 3", got)
	}
	if h.OwnerClient != "" {
		t.Errorf("OwnerClient = %q, want empty (system-owned)", h.OwnerClient)
	}
	if h.Credentials.APIToken != "platforms/proxmox/api.token" {
		t.Errorf("Credentials.APIToken = %q", h.Credentials.APIToken)
	}
}

func TestRegistry_Get_WardenclyffeKind(t *testing.T) {
	r := host.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	h := r.Get("server2")
	if h == nil {
		t.Fatal("Get(server2) = nil; want non-nil")
	}
	if h.Kind != host.KindWardenclyffe {
		t.Errorf("Kind = %q, want %q", h.Kind, host.KindWardenclyffe)
	}
	if h.OwnerClient != "acme-corp" {
		t.Errorf("OwnerClient = %q, want acme-corp", h.OwnerClient)
	}
}

func TestRegistry_Get_Unknown(t *testing.T) {
	r := host.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if h := r.Get("does-not-exist"); h != nil {
		t.Errorf("Get(unknown) = %+v; want nil", h)
	}
}

func TestRegistry_Load_MissingIndexIsEmpty(t *testing.T) {
	tmp := t.TempDir()
	// No hosts.yaml, no hosts/ — Load should succeed with empty result.
	r := host.NewRegistry(tmp)
	if err := r.Load(); err != nil {
		t.Fatalf("Load on empty dir: %v", err)
	}
	if got := r.Count(); got != 0 {
		t.Errorf("Count() = %d, want 0", got)
	}
}

func TestRegistry_Load_MalformedYAMLFails(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "hosts.yaml"), []byte("not: valid: yaml: ::"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := host.NewRegistry(tmp)
	err := r.Load()
	if err == nil {
		t.Fatal("Load on malformed YAML succeeded; want error")
	}
}

func TestNewConnectorFor_RegisteredKinds(t *testing.T) {
	// The kinds/ packages register themselves at init. Without importing
	// them this test would fail with ErrUnknownKind. We re-import via
	// blank import in registry_test_kinds.go.
	for _, k := range []host.Kind{host.KindProxmox, host.KindWardenclyffe, host.KindBare} {
		h := &host.Host{Name: "t-" + string(k), Kind: k}
		c, err := host.NewConnectorFor(h)
		if err != nil {
			t.Errorf("NewConnectorFor(kind=%q): %v", k, err)
			continue
		}
		if c == nil {
			t.Errorf("NewConnectorFor(kind=%q) = nil", k)
		}
	}
}

func TestNewConnectorFor_UnknownKindReturnsErr(t *testing.T) {
	h := &host.Host{Name: "ghost", Kind: host.Kind("does-not-exist")}
	_, err := host.NewConnectorFor(h)
	if !errors.Is(err, host.ErrUnknownKind) {
		t.Errorf("err = %v; want ErrUnknownKind", err)
	}
}
