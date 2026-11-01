package client_test

import (
	"os"
	"path/filepath"
	"testing"

	"warden/internal/client"
)

const fixtureRoot = "testdata"

func TestRegistry_Load_FixturesParse(t *testing.T) {
	r := client.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := r.Count(), 2; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
}

func TestRegistry_Get_PremiumTenant(t *testing.T) {
	r := client.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := r.Get("acme-corp")
	if c == nil {
		t.Fatal("Get(acme-corp) = nil; want non-nil")
	}
	if c.Slug != "acme-corp" {
		t.Errorf("Slug = %q, want acme-corp", c.Slug)
	}
	if c.Tier != client.TierPremium {
		t.Errorf("Tier = %q, want %q", c.Tier, client.TierPremium)
	}
	if !c.Tier.IsKnown() {
		t.Errorf("Tier.IsKnown() = false")
	}
	if c.IsSystem() {
		t.Errorf("IsSystem() = true; want false (acme-corp is a tenant)")
	}
	if got := len(c.HostsOwned); got != 1 {
		t.Errorf("len(HostsOwned) = %d, want 1", got)
	}
}

func TestRegistry_Get_DefaultTenant(t *testing.T) {
	r := client.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := r.Get("example-tenant")
	if c == nil {
		t.Fatal("Get(example-tenant) = nil; want non-nil")
	}
	if c.Tier != client.TierDefault {
		t.Errorf("Tier = %q, want %q", c.Tier, client.TierDefault)
	}
}

func TestRegistry_Get_SystemSlugIsImplicit(t *testing.T) {
	r := client.NewRegistry(fixtureRoot)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// System tenant is implicit; the registry never returns it.
	if c := r.Get(""); c != nil {
		t.Errorf("Get(\"\") = %+v; want nil", c)
	}
	if c := r.Get("system"); c != nil {
		t.Errorf("Get(\"system\") = %+v; want nil", c)
	}
}

func TestRegistry_Load_MissingIndexIsEmpty(t *testing.T) {
	tmp := t.TempDir()
	r := client.NewRegistry(tmp)
	if err := r.Load(); err != nil {
		t.Fatalf("Load on empty dir: %v", err)
	}
	if r.Count() != 0 {
		t.Errorf("Count() = %d, want 0", r.Count())
	}
}

func TestRegistry_Load_UnknownTierFails(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "clients", "bad"), 0o755); err != nil {
		t.Fatal(err)
	}
	idx := []byte("version: 1\nclients:\n  - slug: bad\n")
	if err := os.WriteFile(filepath.Join(tmp, "clients.yaml"), idx, 0o644); err != nil {
		t.Fatal(err)
	}
	bad := []byte("slug: bad\nname: Bad\nowner_email: a@b\ntier: invalid-tier\n")
	if err := os.WriteFile(filepath.Join(tmp, "clients", "bad", "client.yaml"), bad, 0o644); err != nil {
		t.Fatal(err)
	}

	r := client.NewRegistry(tmp)
	if err := r.Load(); err == nil {
		t.Fatal("Load with unknown tier succeeded; want error")
	}
}

func TestPolicy_Has(t *testing.T) {
	p := &client.Policy{
		Capabilities: []client.Capability{
			client.CapHostRead,
			client.CapDeployRun,
		},
	}
	if !p.Has(client.CapHostRead) {
		t.Error("Has(CapHostRead) = false; want true")
	}
	if !p.Has(client.CapDeployRun) {
		t.Error("Has(CapDeployRun) = false; want true")
	}
	if p.Has(client.CapMemorySearchGlobal) {
		t.Error("Has(CapMemorySearchGlobal) = true; want false (not granted)")
	}

	wildcard := &client.Policy{Capabilities: []client.Capability{"*"}}
	if !wildcard.Has(client.CapMemorySearchGlobal) {
		t.Error("wildcard policy: Has(any) = false; want true")
	}

	if (*client.Policy)(nil).Has(client.CapHostRead) {
		t.Error("nil policy: Has(any) = true; want false")
	}
}

func TestSystemPolicy_GrantsAll(t *testing.T) {
	if !client.SystemPolicy.Has(client.CapMemorySearchGlobal) {
		t.Error("SystemPolicy: Has(CapMemorySearchGlobal) = false; want true")
	}
	if !client.SystemPolicy.Has(client.CapHostWrite) {
		t.Error("SystemPolicy: Has(CapHostWrite) = false; want true")
	}
}
