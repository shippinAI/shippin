package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type contractMethod struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func TestOpenAPIMethodContract(t *testing.T) {
	spec, py := specFile(), pyYAML(t)
	rec := &recorder{patterns: servedPatterns(t)}
	if dest := os.Getenv("CONTRACT_ROUTES_OUT"); dest != "" {
		b, _ := json.MarshalIndent(rec.patterns, "", "  ")
		if err := os.WriteFile(dest, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(py, filepath.Join(filepath.Dir(spec), "tests", "contract_routes.py"), spec)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("local contract resolution: %v: %s", err, raw)
	}
	var paths map[string]map[string]contractMethod
	if err := json.Unmarshal(raw, &paths); err != nil {
		t.Fatal(err)
	}
	code := map[string]bool{}
	for _, p := range rec.patterns {
		method, path, _ := strings.Cut(p, " ")
		if strings.HasPrefix(path, "/v1/") {
			code[method+" "+norm(path)] = true
		}
	}
	specified := map[string]bool{}
	documentedPath := map[string]bool{}
	for path, methods := range paths {
		documentedPath[norm(path)] = true
		for method, meta := range methods {
			key := method + " " + norm(path)
			specified[key] = true
			switch meta.Status {
			case "planned":
				if meta.Reason == "" {
					t.Errorf("planned operation lacks reason: %s", key)
				}
				if code[key] {
					t.Errorf("stale planned marker: %s", key)
				}
			case "registered", "blocked":
				if !code[key] {
					t.Errorf("documented method not registered (mark intended work explicitly planned): %s", key)
				}
				if meta.Status == "blocked" && meta.Reason == "" {
					t.Errorf("blocked operation lacks reason: %s", key)
				}
			default:
				t.Errorf("unknown implementation status %q: %s", meta.Status, key)
			}
		}
	}
	// Existing path exceptions remain governed by TestOpenAPIContract. Once a path is
	// documented, every registered method must be described; no method allowlist.
	for key := range code {
		_, path, _ := strings.Cut(key, " ")
		if documentedPath[path] && !specified[key] {
			t.Errorf("registered method missing from contract: %s", key)
		}
	}
}
