package api

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/db"
)

// expectations parses testdata/staff-matrix/*.txt (read in name order, each file <= 150 lines):
// role -> allowed route patterns.
func expectations(t *testing.T) map[string][]string {
	t.Helper()
	files, _ := filepath.Glob("testdata/staff-matrix/*.txt")
	if len(files) == 0 {
		t.Fatal("no testdata/staff-matrix/*.txt")
	}
	slices.Sort(files)
	var all []string
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, string(b))
	}
	out, role := map[string][]string{}, ""
	sc := bufio.NewScanner(strings.NewReader(strings.Join(all, "\n")))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case l == "" || strings.HasPrefix(l, "#"):
		case strings.HasPrefix(l, "["):
			role = strings.Trim(l, "[]")
			out[role] = []string{}
		default:
			out[role] = append(out[role], l)
		}
	}
	return out
}

// stubMux mounts every operator route with its real permission but a stub handler (204), so
// only the gate decides.
func stubMux(a *API) *http.ServeMux {
	mux := http.NewServeMux()
	routes := []opRoute{}
	for _, rt := range a.operatorRoutes() {
		routes = append(routes, opRoute{rt.pattern, rt.perm, func(w http.ResponseWriter, _ *http.Request, _ *caller) { w.WriteHeader(204) }})
	}
	a.mountOperator(mux, routes)
	return mux
}

func call(mux *http.ServeMux, pattern, user string, iam bool) int {
	method, path, _ := strings.Cut(pattern, " ")
	path = strings.NewReplacer("{userId}", "u9", "{roleKey}", "staff.x", "{state}", "deactivate", "{requestId}", "q1",
		"{decision}", "approve", "{permKey}", "catalog.plans.read").Replace(pathVars.Replace(path))
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-Test-User", user)
	if iam {
		req.Header.Set("X-Test-IAM", "1")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// Per preset role x per route, against the hand-written file; everything unlisted is 403.
func TestStaffMatrixDenyByDefault(t *testing.T) {
	exp := expectations(t)
	if len(exp) != len(db.StaffPresets) {
		t.Fatalf("expectation file has %d roles, presets %d", len(exp), len(db.StaffPresets))
	}
	for _, p := range db.StaffPresets {
		allowed, ok := exp[p.Key]
		if !ok {
			t.Fatalf("no expectations for %s", p.Key)
		}
		idp := newFakeIdP().add("u1", p.Key)
		a := staffAPI(newFakeStore("enforce"), idp)
		mux := stubMux(a)
		for _, rt := range a.operatorRoutes() {
			want := 403
			if slices.Contains(allowed, "ALL") || slices.Contains(allowed, rt.pattern) {
				want = 204
			}
			if got := call(mux, rt.pattern, "u1", false); got != want {
				t.Errorf("%s %s: got %d, want %d", p.Key, rt.pattern, got, want)
			}
		}
	}
}

// The file may only name routes that exist, so a typo can't silently pass.
func TestStaffMatrixNamesRealRoutes(t *testing.T) {
	have := map[string]bool{}
	for _, rt := range staffAPI(newFakeStore("enforce"), newFakeIdP()).operatorRoutes() {
		have[rt.pattern] = true
	}
	for role, routes := range expectations(t) {
		for _, r := range routes {
			if r != "ALL" && !have[r] {
				t.Errorf("%s lists unknown route %q", role, r)
			}
		}
	}
}

// Callers who must get nothing: no roles, no MFA, unknown role key, a role whose permissions
// are outside the catalog, and (enforce, no fallback) an IAM owner without staff roles.
func TestStaffNobodyElseGetsAnything(t *testing.T) {
	store := newFakeStore("enforce")
	store.roles = append(store.roles, db.StaffRole{Key: "staff.bogus", Name: "Bogus", Permissions: []string{"tenants.everything", "root"}})
	idp := newFakeIdP().add("none").add("nomfa", "staff.owner").add("unknown", "staff.ghost").add("bogus", "staff.bogus").add("iam")
	idp.mfa["nomfa"] = false
	a := staffAPI(store, idp)
	mux := stubMux(a)
	for _, u := range []string{"none", "nomfa", "unknown", "bogus", "iam"} {
		for _, rt := range a.operatorRoutes() {
			if got := call(mux, rt.pattern, u, u == "iam"); got != 403 {
				t.Errorf("%s %s: got %d, want 403", u, rt.pattern, got)
			}
		}
	}
	if !slices.Contains(store.actions(), "authz.denied") {
		t.Error("refusals must be audited as authz.denied")
	}
}
