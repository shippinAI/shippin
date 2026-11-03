package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
)

// With operator.origin_only, every operator route answers 404 off the operator origin (before
// any token or role check) and is gated as usual on it.
func TestOperatorAPIOnlyOnItsOrigin(t *testing.T) {
	a := gateAPI("")
	a.cfg.Operator.Host, a.cfg.Operator.OriginOnly = "ops.example.test", true
	a.flow = auth.NewFlow(a.cfg, nil)
	mux := http.NewServeMux()
	a.registerOperator(mux)
	for _, rt := range a.operatorRoutes() {
		method, path, _ := strings.Cut(rt.pattern, " ")
		for host, want := range map[string]int{"console.example.test": 404, "ops.example.test": 403} {
			req := httptest.NewRequest(method, pathVars.Replace(path), strings.NewReader("{}"))
			req.Host = host
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != want {
				t.Errorf("%s on %s: got %d, want %d", rt.pattern, host, rec.Code, want)
			}
		}
	}
}
