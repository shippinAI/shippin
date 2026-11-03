package hostd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Path ids are validated before any handler (and so before any incus call) runs.
func TestRoutesRejectBadIDs(t *testing.T) {
	mux := testServer().Routes()
	for _, p := range []string{
		"POST /tenants/x/provision", "DELETE /tenants/..%2f..", "POST /tenants/00000000-0000-4000-8000-000000000001/workspaces/abc",
		"POST /tenants/00000000-0000-4000-8000-000000000001/workspaces/abc/delete",
		"GET /tenants/00000000-0000-4000-8000-000000000001/workspaces/x;id/listening",
	} {
		m, path, _ := strings.Cut(p, " ")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(m, path, strings.NewReader("{}")))
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d", p, rec.Code)
		}
	}
}

func TestActionRejectsBadBody(t *testing.T) {
	mux := testServer().Routes()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/tenants/00000000-0000-4000-8000-000000000001/workspaces/01a1414a-4346-7d6c-ab90-87332303e90d/stop", strings.NewReader("{"))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestActionHostChecks(t *testing.T) {
	s := testServer()
	for _, a := range []string{"domains", "previews"} {
		err := s.Action(t.Context(), "00000000-0000-4000-8000-000000000001", "01a1414a-4346-7d6c-ab90-87332303e90d", a, WorkspaceReq{Host: "console.example.test"})
		if err == nil || !strings.Contains(err.Error(), "invalid workspace host") {
			t.Errorf("%s with a foreign host: %v", a, err)
		}
	}
	if err := s.Action(t.Context(), "t", "w", "rm -rf", WorkspaceReq{}); err == nil {
		t.Error("unknown action accepted")
	}
}

func TestRunAllowlist(t *testing.T) {
	for _, bin := range []string{"sh", "bash", "curl", "/usr/bin/incus", "incus "} {
		if _, err := run(t.Context(), "", bin, "x"); err == nil || err.Error() != "binary not allowed" {
			t.Errorf("%q allowed: %v", bin, err)
		}
	}
}
