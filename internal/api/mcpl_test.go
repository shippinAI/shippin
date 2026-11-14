package api

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/mcpl"
)

// TestMCPLConformance holds the API to docs/mcpl.md. Every generated action is served on its derived
// route behind its own permission. A route that carries something else (a stream, a socket, a
// webhook) is mounted with a.transport and says so. Every other served /v1 route is legacy, listed in
// testdata/mcpl-undescribed.txt; that list may only shrink, as entities move into
// internal/<domain>/<entity>/<verb>.go. Regenerate it with UPDATE_MCPL=1.
func TestMCPLConformance(t *testing.T) {
	a := servedAPI()
	rec := &patternRec{mux: http.NewServeMux()}
	a.Register(rec)
	served := map[string]bool{}
	for _, pat := range rec.all {
		if _, path, ok := strings.Cut(pat, " "); ok && strings.HasPrefix(path, "/v1/") {
			served[pat] = true
		}
	}
	gate := map[string]string{}
	for _, rt := range a.operatorRoutes() {
		gate[rt.pattern] = rt.perm
	}
	actions := describedActions()
	for _, err := range mcpl.Validate(actions) {
		t.Error(err)
	}
	described := map[string]bool{"GET /v1/mcpl": true, "GET /v1/operator/mcpl": true} // the contract's own manifests
	for _, x := range actions {
		described[x.Route] = true
		if !served[x.Route] {
			t.Errorf("%s: %s is not served", x.ID, x.Route)
		}
		if perm, ok := gate[x.Route]; x.Access == mcpl.Staff && (!ok || perm != x.Perm) {
			t.Errorf("%s: route gate %q, the action needs %q", x.ID, perm, x.Perm)
		}
	}
	for pat, carries := range a.transports {
		if !served[pat] && strings.HasPrefix(strings.SplitN(pat, " ", 2)[1], "/v1/") {
			t.Errorf("transport %s (%s) is not served", pat, carries)
		}
		if described[pat] {
			t.Errorf("%s is both an action and a transport", pat)
		}
		described[pat] = true
	}
	var legacy []string
	for pat := range served {
		if !described[pat] {
			legacy = append(legacy, pat)
		}
	}
	ratchet(t, "testdata/mcpl-undescribed.txt", legacy, "UPDATE_MCPL", "served /v1 routes that are not yet generated actions",
		"write it as an action file instead (docs/mcpl.md)")
	t.Logf("MCPL: %d generated actions, %d legacy routes", len(actions), len(legacy))
}

// ratchet fails on a new entry or a stale one, so a known-gaps list can only shrink. env=1 rewrites it.
func ratchet(t *testing.T, file string, got []string, env, what, hint string) {
	t.Helper()
	slices.Sort(got)
	if os.Getenv(env) == "1" {
		_ = os.WriteFile(file, []byte("# "+what+". This list may only shrink: move an entity to internal/<domain>/<entity>/<verb>.go, then "+env+"=1 go test ./internal/api\n"+strings.Join(got, "\n")+"\n"), 0o644)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%s: %v (create it with %s=1)", file, err, env)
	}
	known := map[string]bool{}
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			known[l] = true
		}
	}
	have := map[string]bool{}
	for _, g := range got {
		have[g] = true
		if !known[g] {
			t.Errorf("new entry %s in %s: %s", g, file, hint)
		}
	}
	for k := range known {
		if !have[k] {
			t.Errorf("%s lists %s, which is gone or generated now; rewrite it with %s=1", file, k, env)
		}
	}
}
