package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The host detection action is served with its real handler behind the operator gate: a role without
// infra.hosts.write is refused before anything is dialled, even though the action only reads.
func TestHostDetectionNeedsThePermissionToAddHosts(t *testing.T) {
	idp := newFakeIdP().add("infra", "staff.infra").add("ops", "staff.ops").add("auditor", "staff.auditor").add("none")
	a := staffAPI(newFakeStore("enforce"), idp)
	mux := http.NewServeMux()
	a.mountOperator(mux, a.operatorRoutes())
	for user, want := range map[string]int{
		"ops":     http.StatusForbidden, // infra.read and guest power, not adding hosts
		"auditor": http.StatusForbidden,
		"none":    http.StatusForbidden,
		"infra":   http.StatusServiceUnavailable, // past the gate; this fake API has no database
	} {
		req := httptest.NewRequest("GET", "/v1/operator/infrastructure/host-detection?address=127.0.0.1:1", nil)
		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s: got %d want %d: %s", user, rec.Code, want, rec.Body.String())
		}
	}
}
