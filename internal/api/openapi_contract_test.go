package api

import (
	"bufio"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/identity"
)

// recorder captures registered patterns (Router) without serving anything.
type recorder struct{ patterns []string }

func (r *recorder) HandleFunc(p string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, p)
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// norm maps "/v1/tenants/{tenantId}/x" and "/tenants/{id}/x" to "/tenants/{}/x".
func norm(p string) string { return pathParam.ReplaceAllString(strings.TrimPrefix(p, "/v1"), "{}") }

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

// TestOpenAPIContract holds each served path to one owner. An action's path is documented by
// api/openapi/actions.gen.json, generated from the action, and the hand-written openapi.yaml must not
// document it too. Every other served path is in openapi.yaml, except those in
// testdata/openapi-drift.txt, which may only shrink (UPDATE_OPENAPI=1 rewrites it). Which methods
// openapi.yaml marks registered or planned is TestOpenAPIMethodContract's.
func TestOpenAPIContract(t *testing.T) {
	actionPaths := map[string]bool{}
	for _, g := range actions.All {
		_, p, _ := strings.Cut(g.Route, " ")
		actionPaths[norm(p)] = true
	}
	spec := specPaths(t, specFile())
	for _, p := range spec {
		if actionPaths[p] {
			t.Errorf("openapi.yaml documents %s, which is an action's: actions.gen.json owns it, remove it from openapi.yaml", p)
		}
	}
	var undocumented []string
	seen := map[string]bool{}
	for _, pat := range servedPatterns(t) {
		_, p, _ := strings.Cut(pat, " ")
		if p = norm(p); actionPaths[p] || seen[p] || slices.Contains(spec, p) {
			continue
		}
		seen[p] = true
		undocumented = append(undocumented, "code "+p)
	}
	ratchet(t, "testdata/openapi-drift.txt", undocumented, "UPDATE_OPENAPI",
		"served paths that are neither actions nor in api/openapi/openapi.yaml",
		"document it in openapi.yaml, or better, make it an action, whose contract is generated")
}

// specFile is the hand-written contract: OPENAPI_SPEC, or the repository's own.
func specFile() string {
	if f := os.Getenv("OPENAPI_SPEC"); f != "" {
		return f
	}
	return filepath.Join("..", "..", "api", "openapi", "openapi.yaml")
}

// servedAPI is the API with everything mounted that an installation can mount.
func servedAPI() *API {
	a := gateAPI("op_superadmin")
	a.flow = &auth.Flow{}
	a.directory = identity.New(nil, a.cfg, nil, nil) // so local-account routes are mounted too
	return a
}

// servedPatterns are the served "METHOD /v1/..." patterns.
func servedPatterns(t *testing.T) []string {
	t.Helper()
	rec := &patternRec{mux: http.NewServeMux()}
	servedAPI().Register(rec)
	var out []string
	for _, pat := range rec.all {
		if _, path, ok := strings.Cut(pat, " "); ok && strings.HasPrefix(path, "/v1/") {
			out = append(out, pat)
		}
	}
	return out
}

// pyYAML is a Python with PyYAML for the checks that resolve the spec's references; without one they
// are skipped, and say so.
func pyYAML(t *testing.T) string {
	t.Helper()
	for _, py := range []string{"python3", "python"} {
		if exec.Command(py, "-c", "import yaml").Run() == nil {
			return py
		}
	}
	t.Skip("no python with PyYAML: the hand-written spec's method check needs it")
	return ""
}
