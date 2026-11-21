package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// An access key is a person's local session for a client: unknown keys are refused, and a known one
// carries the person and the sign-in method of the session that made it. It never reaches the IdP.
func TestRequireAcceptsAccessKeys(t *testing.T) {
	f := &Flow{Keys: func(_ context.Context, tok string) (string, string, bool) {
		return "u1", "passkey", tok == "shp_good"
	}}
	var got struct {
		user, method, idp string
		viaKey            bool
	}
	h := f.Require(func(w http.ResponseWriter, r *http.Request) {
		got.user, got.method, got.idp, got.viaKey = LocalUser(r.Context()), SignInMethod(r.Context()), FromContext(r.Context()), ViaAccessKey(r.Context())
	})
	for tok, want := range map[string]int{"shp_good": 200, "shp_bad": 401} {
		req := httptest.NewRequest("GET", "/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", tok, rec.Code, want)
		}
	}
	if got.user != "u1" || got.method != "passkey" || got.idp != "" || !got.viaKey {
		t.Errorf("admitted as %+v", got)
	}
	req := httptest.NewRequest("GET", "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer shp_good")
	rec := httptest.NewRecorder()
	(&Flow{}).Require(func(http.ResponseWriter, *http.Request) { t.Error("admitted with no key store") })(rec, req)
	if rec.Code != 401 {
		t.Errorf("no key store: %d", rec.Code)
	}
}
