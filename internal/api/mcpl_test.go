package api

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/identity"
	"shippin.cloud/console/internal/mcpl"
)

// TestMCPLConformance holds the API to docs/mcpl.md:
//   - every described action follows the rules and is really served, with the permission that gates it;
//   - every served /v1 route is described, or listed in testdata/mcpl-undescribed.txt;
//   - every staff change takes a reason, or is listed in testdata/mcpl-no-reason.txt.
//
// Both lists may only shrink. Regenerate them with UPDATE_MCPL=1 after describing more routes.
func TestMCPLConformance(t *testing.T) {
	a := gateAPI("op_superadmin")
	a.flow = &auth.Flow{}
	a.directory = identity.New(nil, a.cfg, nil, nil) // so local-account routes are mounted too
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

	actions := a.actions()
	for _, err := range mcpl.Validate(actions) {
		t.Error(err)
	}
	described := map[string]bool{}
	var noReason []string
	for _, x := range actions {
		described[x.Route] = true
		if !served[x.Route] {
			t.Errorf("%s describes %s, which is not served", x.ID, x.Route)
		}
		if perm, ok := gate[x.Route]; ok && (x.Access != mcpl.Staff || x.Perm != perm) {
			t.Errorf("%s: route is gated by %q, the action says %s %q", x.ID, perm, x.Access, x.Perm)
		}
		if x.Access == mcpl.Staff && x.Risk != mcpl.Read && !x.Reason {
			noReason = append(noReason, x.ID)
		}
	}
	var undescribed []string
	for pat := range served {
		if !described[pat] {
			undescribed = append(undescribed, pat)
		}
	}
	ratchet(t, "testdata/mcpl-undescribed.txt", undescribed, "served /v1 routes with no MCPL action")
	ratchet(t, "testdata/mcpl-no-reason.txt", noReason, "staff changes that take no reason")
	t.Logf("MCPL: %d actions described, %d routes still undescribed", len(actions), len(undescribed))
}

// ratchet fails on a new entry or a stale one, so a known-gaps list can only shrink.
func ratchet(t *testing.T, file string, got []string, what string) {
	t.Helper()
	slices.Sort(got)
	if os.Getenv("UPDATE_MCPL") == "1" {
		_ = os.WriteFile(file, []byte("# "+what+". This list may only shrink: describe one, then UPDATE_MCPL=1 go test -run TestMCPLConformance ./internal/api\n"+strings.Join(got, "\n")+"\n"), 0o644)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%s: %v (create it with UPDATE_MCPL=1)", file, err)
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
			t.Errorf("new %s: %s (describe it per docs/mcpl.md)", strings.TrimSuffix(what, "s"), g)
		}
	}
	for k := range known {
		if !have[k] {
			t.Errorf("%s lists %s, which is fixed now; regenerate with UPDATE_MCPL=1", file, k)
		}
	}
}
