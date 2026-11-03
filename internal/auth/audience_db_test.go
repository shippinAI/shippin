package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/db"
)

// A console sign-in is never accepted on the operator origin and an operator sign-in never on
// the console (bearer tokens are unaffected).
func TestSessionsStayOnTheirOrigin(t *testing.T) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	d, err := db.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	st, err := NewStore(d, strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	f := NewFlow(opsCfg("ops-app"), st)
	exp := time.Now().Add(time.Hour)
	consoleID, _ := st.New(t.Context(), &Session{AccessToken: "console-tok", Expires: exp})
	opsID, _ := st.New(t.Context(), &Session{AccessToken: "ops-tok", Aud: AudOperator, Expires: exp})
	cases := []struct{ host, sid, want string }{
		{"console.example.test", consoleID, "console-tok"}, {"ops.example.test", consoleID, ""},
		{"ops.example.test", opsID, "ops-tok"}, {"console.example.test", opsID, ""},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		r.Host = c.host
		r.AddCookie(&http.Cookie{Name: cookieName, Value: c.sid})
		if got := f.Token(r); got != c.want {
			t.Errorf("%s with %s session: token %q, want %q", c.host, c.want, got, c.want)
		}
		if got := f.SessionToken(r); got != c.want {
			t.Errorf("SessionToken on %s: %q", c.host, got)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	r.Host = "ops.example.test"
	r.Header.Set("Authorization", "Bearer pat")
	if f.Token(r) != "pat" {
		t.Error("bearer tokens are accepted on either origin; the IdP and the gate check them")
	}
}
