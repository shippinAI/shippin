package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
)

type patternRec struct {
	mux *http.ServeMux
	all []string
}

func (p *patternRec) HandleFunc(pat string, h func(http.ResponseWriter, *http.Request)) {
	p.all = append(p.all, pat)
	p.mux.HandleFunc(pat, h)
}

// deploy/tests/unauth-routes.txt is the full sweep list for gc-unauth-401.sh: every /v1 route that
// answers exactly 401 without a session. Regenerate with UPDATE_UNAUTH=1 go test -run TestUnauthRoutes.
func TestUnauthRoutesFile(t *testing.T) {
	a := gateAPI("op_superadmin")
	a.flow = &auth.Flow{}
	rec := &patternRec{mux: http.NewServeMux()}
	a.Register(rec)
	var got []string
	for _, pat := range rec.all {
		method, path, ok := strings.Cut(pat, " ")
		if !ok || !strings.HasPrefix(path, "/v1/") {
			continue
		}
		if code := unauthCode(rec.mux, method, pathVars.Replace(path)); code == http.StatusUnauthorized {
			got = append(got, method+" "+path)
		}
	}
	slices.Sort(got)
	const f = "../../deploy/tests/unauth-routes.txt"
	if os.Getenv("UPDATE_UNAUTH") == "1" {
		_ = os.WriteFile(f, []byte(strings.Join(got, "\n")+"\n"), 0o644)
	}
	raw, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(got, "\n") + "\n"; string(raw) != want {
		t.Fatalf("%s is stale; rerun with UPDATE_UNAUTH=1", f)
	}
	for _, must := range []string{"GET /v1/tenants/{tenantId}/workspaces/{workspaceId}/discover",
		"POST /v1/tenants/{tenantId}/workspaces/{workspaceId}/adopt", "POST /v1/tenants/{tenantId}/projects/{projectId}/github/publish",
		"POST /v1/operator/tenants/{tenantId}/projects", "GET /v1/operator/tenants/{tenantId}/workspaces/{workspaceId}/discover",
		"POST /v1/tenants/{tenantId}/github/import"} {
		if !slices.Contains(got, must) {
			t.Errorf("%s does not answer 401 without a session", must)
		}
	}
}

func unauthCode(mux *http.ServeMux, method, path string) (code int) {
	defer func() {
		if recover() != nil {
			code = -1 // a public handler ran into the nil test DB: not a 401 route
		}
	}()
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader("{}")))
	return rr.Code
}
