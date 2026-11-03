package api

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// recorder captures registered patterns (Router) without serving anything.
type recorder struct{ patterns []string }

func (r *recorder) HandleFunc(p string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, p)
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// norm maps "/v1/tenants/{tenantId}/x" and "/tenants/{id}/x" to "/tenants/{}/x".
func norm(p string) string { return pathParam.ReplaceAllString(strings.TrimPrefix(p, "/v1"), "{}") }

// codePaths are the registered /v1 API paths (method dropped), normalised.
func codePaths(t *testing.T) []string {
	rec := &recorder{}
	(&API{cfg: testCfg()}).Register(rec)
	out := []string{}
	for _, p := range rec.patterns {
		_, path, _ := strings.Cut(p, " ")
		if strings.HasPrefix(path, "/v1/") && !slices.Contains(out, norm(path)) {
			out = append(out, norm(path))
		}
	}
	return out
}

// specPaths reads the top-level `paths:` keys of openapi.yaml (two-space indented "/x:" lines).
func specPaths(t *testing.T, file string) []string {
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	key := regexp.MustCompile(`^  (/[^:]*):\s*$`)
	out, in := []string{}, false
	for sc := bufio.NewScanner(f); sc.Scan(); {
		l := sc.Text()
		if l == "paths:" {
			in = true
			continue
		}
		if in && l != "" && l[0] != ' ' {
			break
		}
		if m := key.FindStringSubmatch(l); in && m != nil {
			out = append(out, norm(m[1]))
		}
	}
	return out
}

// TestOpenAPIContract: every API route is in the spec and every spec path is served, except the
// recorded drift (testdata/openapi-drift.txt: "code <path>" or "spec <path>"), which may only
// shrink. Set OPENAPI_SPEC to the openapi.yaml path.
func TestOpenAPIContract(t *testing.T) {
	file := os.Getenv("OPENAPI_SPEC")
	if file == "" {
		t.Skip("OPENAPI_SPEC not set")
	}
	allowed := map[string]bool{}
	raw, _ := os.ReadFile(filepath.Join("testdata", "openapi-drift.txt"))
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" && l[0] != '#' {
			allowed[l] = true
		}
	}
	code, spec := codePaths(t), specPaths(t, file)
	seen := map[string]bool{}
	check := func(kind string, have, other []string) {
		for _, p := range have {
			if slices.Contains(other, p) {
				continue
			}
			line := kind + " " + p
			seen[line] = true
			if !allowed[line] {
				t.Errorf("new drift: %s (add it to the spec, or to testdata/openapi-drift.txt with a reason)", line)
			}
		}
	}
	check("code", code, spec)
	check("spec", spec, code)
	for l := range allowed {
		if !seen[l] {
			t.Errorf("stale drift entry (now in sync, remove it): %s", l)
		}
	}
}
