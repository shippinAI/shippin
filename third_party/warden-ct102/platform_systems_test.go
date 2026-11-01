package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlugifyID(t *testing.T) {
	if g := slugifyID("  My Service! "); g != "my-service" {
		t.Fatalf("got %q", g)
	}
}

func TestPlatformSystemPersistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WARDEN_STATE_DIR", dir)
	platformPath = filepath.Join(dir, "platform_systems.json")
	platformSystems = nil
	loadPlatformSystems()
	if len(platformSystems) < 3 {
		t.Fatalf("expected default seed, got %d", len(platformSystems))
	}
	ps := PlatformSystem{
		ID:        "test-app",
		Name:      "Test App",
		Category:  "app",
		Scope:     "internal",
		CheckHost: "127.0.0.1",
		CheckPort: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := appendPlatformSystem(ps); err != nil {
		t.Fatal(err)
	}
	if err := removePlatformSystem("test-app"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(platformPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"systems"`) {
		t.Fatalf("unexpected file: %s", string(data))
	}
}
