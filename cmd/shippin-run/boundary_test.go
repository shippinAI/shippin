package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The older file store is a compatibility tool, not a second tenant authority.
// Check the actual executable dependency closure, including transitive imports.
func TestConsoleDoesNotDependOnLegacyFleetStore(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "./cmd/console")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("console dependency inspection: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if dependency == "shippin.cloud/console/internal/runtime/runfleet" {
			t.Fatal("console depends on the unscoped legacy fleet store; use the canonical runtime runner owners")
		}
	}
}
