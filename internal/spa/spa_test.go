package spa

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownAPIIs404ProblemAndSPAFallbackStays(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := Handler(dir)
	for _, p := range []string{"/v1", "/v1/", "/v1/nope", "/v1/projects/x/unknown"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/problem+json" {
			t.Fatalf("%s: got %d %q", p, rec.Code, rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), `"status":404`) {
			t.Fatalf("%s: body %s", p, rec.Body.String())
		}
	}
	for _, p := range []string{"/", "/projects/abc", "/v1x", "/v2/thing"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "spa") {
			t.Fatalf("%s: SPA fallback lost: %d", p, rec.Code)
		}
	}
}
