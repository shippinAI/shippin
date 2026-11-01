package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCatalog_AllRepoTemplatesParse asserts that every yml in the repo's
// templates/compose/ directory parses cleanly. Spec 07 §12 requires this.
//
// This test runs from the warden/ directory (Go's default test cwd) and
// expects ../templates/compose/ to exist (the repo dev path). If you've
// moved templates to /etc/warden/... only, set WARDEN_CATALOG_DIR before
// running tests.
func TestCatalog_AllRepoTemplatesParse(t *testing.T) {
	dir := os.Getenv("WARDEN_CATALOG_DIR")
	if dir == "" {
		dir = "../templates/compose"
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("catalog dir %q not present; skipping (set WARDEN_CATALOG_DIR to override)", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read catalog dir: %v", err)
	}

	yamls := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		yamls++
		path := filepath.Join(dir, name)

		t.Run(name, func(t *testing.T) {
			tmpl, err := parseCatalogTemplate(path)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			if tmpl.Name == "" {
				t.Errorf("%s: x-warden.name is empty", path)
			}
			if tmpl.Category == "" {
				t.Errorf("%s: x-warden.category is empty", path)
			}
			if tmpl.Version == "" {
				t.Errorf("%s: x-warden.version is empty", path)
			}
			if tmpl.Source.Type == "" {
				t.Errorf("%s: x-warden.source.type is empty", path)
			}
		})
	}

	if yamls == 0 {
		t.Errorf("no yml/yaml files found in %s — expected at least one", dir)
	}
}

// TestCatalog_LoadInventory smoke-tests the full loadCatalog() path and
// verifies the inventory shape matches what the page template expects.
func TestCatalog_LoadInventory(t *testing.T) {
	dir := os.Getenv("WARDEN_CATALOG_DIR")
	if dir == "" {
		dir = "../templates/compose"
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("catalog dir %q not present; skipping", dir)
	}
	t.Setenv("WARDEN_CATALOG_DIR", dir)

	// Reset cache so we get a fresh load
	invalidateCatalogCache()

	inv := getCatalog()
	if !inv.Available {
		t.Fatalf("inventory not available: %s", inv.Error)
	}
	if inv.Dir != dir {
		t.Errorf("Dir = %q, want %q", inv.Dir, dir)
	}
	if inv.DirSource != "env" {
		t.Errorf("DirSource = %q, want %q", inv.DirSource, "env")
	}
	if len(inv.Templates) == 0 {
		t.Errorf("Templates is empty")
	}
	if inv.OkCount+inv.ErrCount != len(inv.Templates) {
		t.Errorf("OkCount(%d) + ErrCount(%d) != len(Templates)(%d)",
			inv.OkCount, inv.ErrCount, len(inv.Templates))
	}
	if inv.ErrCount > 0 {
		t.Errorf("ErrCount = %d, want 0 (committed templates should all parse)", inv.ErrCount)
	}
	if inv.LoadMillis < 0 {
		t.Errorf("LoadMillis = %d, want >= 0", inv.LoadMillis)
	}
}

// TestCatalog_ResolveDirNotFound asserts honest failure when no dir exists.
func TestCatalog_ResolveDirNotFound(t *testing.T) {
	t.Setenv("WARDEN_CATALOG_DIR", "/does/not/exist/anywhere/promise")

	// Move cwd somewhere that has no ./templates/compose or ../templates/compose
	origWd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	dir, source := resolveCatalogDir()
	if dir != "" {
		t.Errorf("expected empty dir when nothing exists, got %q (source=%q)", dir, source)
	}
	if source != "none" {
		t.Errorf("expected source=none, got %q", source)
	}
}

func TestCatalog_MediaTemplateRequiresMediastackBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jellyfin.yml")
	if err := os.WriteFile(path, []byte(`
services:
  jellyfin:
    image: jellyfin/jellyfin:latest
x-warden:
  name: jellyfin
  version: "0.1.0"
  category: media
  estate: mediastack
  default_visibility: internal
  source:
    type: original
  storage_classes:
    config:
      backup: required
    library:
      backup: optional
    staging:
      backup: never
    cache:
      backup: never
  ports:
    - internal: 8096
      external: 8096
      protocol: http
      public: false
      label: jellyfin
`), 0o600); err != nil {
		t.Fatalf("write media template: %v", err)
	}

	tmpl, err := parseCatalogTemplate(path)
	if err != nil {
		t.Fatalf("parse media template: %v", err)
	}
	if tmpl.Estate != EstateMediastack {
		t.Errorf("Estate = %q, want %q", tmpl.Estate, EstateMediastack)
	}
	if tmpl.DefaultVisibility != VisibilityInternal {
		t.Errorf("DefaultVisibility = %q, want %q", tmpl.DefaultVisibility, VisibilityInternal)
	}
}

func TestCatalog_NonMediaTemplateDefaultsToAIaaS(t *testing.T) {
	tmpl := CatalogTemplate{
		Name:     "authentik",
		Category: "identity",
	}

	if got := estateForTemplate(tmpl); got != EstateAIaaS {
		t.Fatalf("estateForTemplate = %q, want %q", got, EstateAIaaS)
	}
	if got := normalizeEstate(""); got != EstateAIaaS {
		t.Fatalf("normalizeEstate(\"\") = %q, want %q", got, EstateAIaaS)
	}
}

func TestCatalog_MediaTemplateRejectsPublicDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "public-media.yml")
	if err := os.WriteFile(path, []byte(`
services:
  jellyfin:
    image: jellyfin/jellyfin:latest
x-warden:
  name: public-media
  version: "0.1.0"
  category: media
  estate: mediastack
  default_visibility: public
  source:
    type: original
  storage_classes:
    config:
      backup: required
  ports:
    - internal: 8096
      external: 8096
      protocol: http
      public: true
      label: jellyfin
`), 0o600); err != nil {
		t.Fatalf("write media template: %v", err)
	}

	_, err := parseCatalogTemplate(path)
	if err == nil {
		t.Fatalf("expected media template with public default to fail")
	}
	if !strings.Contains(err.Error(), "default_visibility") {
		t.Fatalf("error = %q, want default_visibility failure", err.Error())
	}
}
