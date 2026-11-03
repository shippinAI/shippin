package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/config"
)

func TestDiscoverAndExchange(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issuer":"` + srv.URL + `","authorization_endpoint":"` + srv.URL + `/auth","token_endpoint":"` + srv.URL + `/token","userinfo_endpoint":"` + srv.URL + `/userinfo"}`))
		case "/token":
			if r.FormValue("code_verifier") == "" && r.PostFormValue("code_verifier") == "" {
				http.Error(w, "pkce", 400)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer"}`))
		case "/userinfo":
			_, _ = w.Write([]byte(`{"sub":"user-1","email":"person@example.invalid","email_verified":true,"name":"Person"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	d, err := Discover(context.Background(), srv.Client(), srv.URL)
	if err != nil || d.TokenEndpoint == "" {
		t.Fatal(err)
	}
	up := config.Upstream{ID: "nomen", Issuer: srv.URL, ClientID: "client"}
	url := UpstreamAuthURL(d, up, "https://console.example.invalid/auth/upstream/callback", "state", "verifier")
	if url == "" {
		t.Fatal("empty auth url")
	}
	// oauth2 Exchange reads the form from the request body. The test server checks PostForm.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer","expires_in":3600}`))
		case "/userinfo":
			_, _ = w.Write([]byte(`{"sub":"user-1","email":"person@example.invalid","email_verified":true,"name":"Person"}`))
		default:
			http.NotFound(w, r)
		}
	})
	info, err := ExchangeUpstream(context.Background(), srv.Client(), d, up, "https://console.example.invalid/cb", "code", "verifier", "")
	if err != nil || info.Sub != "user-1" || !info.EmailVerified {
		t.Fatal(err, info)
	}
}
