// vaultix-demo runs hosted Vaultix locally behind the real console sign-in (auth.Flow: OIDC PKCE
// against a real Nomen/Zitadel instance) with the /vaultix console page. Every person who signs
// in gets their own Vaultix organization and workspace. Loopback only; never deploy it.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/spa"
	"shippin.cloud/console/internal/vaultix"
	"shippin.cloud/console/internal/vaultixhost"
)

func env(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("set %s", k)
	}
	return v
}

func secretFile(k string) string {
	b, err := os.ReadFile(env(k))
	if err != nil {
		log.Fatalf("%s: %v", k, err)
	}
	return strings.TrimSpace(string(b))
}

func main() {
	addr := env("VX_DEMO_ADDR")
	if h, _, err := net.SplitHostPort(addr); err != nil || (h != "localhost" && !net.ParseIP(h).IsLoopback()) {
		log.Fatal("VX_DEMO_ADDR must be a loopback host:port")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, env("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	key := make([]byte, 32)
	if _, err := randRead(key); err != nil {
		log.Fatal(err)
	}
	store, err := auth.NewStore(d, hex.EncodeToString(key))
	if err != nil {
		log.Fatal(err)
	}
	cfg := &config.Config{}
	if raw, err := os.ReadFile(env("SHIPPIN_CONFIG")); err != nil || json.Unmarshal(raw, cfg) != nil {
		log.Fatal("SHIPPIN_CONFIG: unreadable console config")
	}
	cfg.ConsoleHost, cfg.Operator = addr, config.Operator{}
	cfg.IDP = config.IDP{Name: cfg.IDP.Name, Issuer: env("NOMEN_ISSUER"), ClientID: env("NOMEN_CLIENT_ID")}
	flow := auth.NewFlow(cfg, store)
	var op struct{ Email, Password string }
	if json.Unmarshal([]byte(secretFile("VAULTIX_OPERATOR_FILE")), &op) != nil {
		log.Fatal("VAULTIX_OPERATOR_FILE: want {\"email\",\"password\"}")
	}
	seal, err := vaultixhost.NewSealer(secretFile("VAULTIX_SEAL_KEY_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	base := env("VAULTIX_URL")
	svc := &vaultixhost.Service{Store: &vaultixhost.Store{Pool: d.Pool}, Op: vaultix.NewOperator(base, op.Email, op.Password, 20*time.Second),
		Seal: seal, Base: base, PublicURL: env("VAULTIX_PUBLIC_URL"), Envs: strings.Split(env("VAULTIX_ENVS"), ","), Timeout: 20 * time.Second}
	who := func(w http.ResponseWriter, r *http.Request) (vaultixhost.Person, bool) {
		id, err := flow.Whoami(r.Context(), auth.FromContext(r.Context()))
		if err != nil || !id.EmailVerified {
			auth.Problem(w, http.StatusUnauthorized, "Sign in with a verified e-mail to use Vaultix.")
			return vaultixhost.Person{}, false
		}
		return vaultixhost.Person{Sub: id.Sub, Email: id.Email, Org: id.OrgID}, true
	}
	mux := http.NewServeMux()
	(&vaultixhost.Handler{Svc: svc, Who: who}).Register(required{mux, flow})
	mux.HandleFunc("GET /auth/login", flow.Login)
	mux.HandleFunc("GET /auth/callback", flow.Callback)
	mux.HandleFunc("GET /auth/logout", flow.Logout)
	mux.HandleFunc("GET /v1/me", flow.Require(func(w http.ResponseWriter, r *http.Request) {
		id, err := flow.Whoami(r.Context(), auth.FromContext(r.Context()))
		if err != nil {
			auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"user_id": id.Sub, "display_name": id.Name, "email": id.Email,
			"platform_roles": []string{}, "tenants": []any{}, "capabilities": map[string]bool{}})
	}))
	mux.HandleFunc("GET /v1/settings/public", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"brand": cfg.Brand, "domain": cfg.Domain, "idp_name": cfg.IDP.Name,
			"idp_host": cfg.IDP.Issuer, "login_ready": true, "app": "console", "console_url": "https://" + addr})
	})
	mux.Handle("/", spa.Handler(env("VX_DEMO_STATIC")))
	log.Printf("vaultix-demo on https://%s (Nomen %s, Vaultix %s)", addr, cfg.IDP.Issuer, base)
	srv := &http.Server{Addr: addr, Handler: sameOrigin(mux), ReadHeaderTimeout: 10 * time.Second, TLSConfig: selfSigned()}
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

// required puts every Vaultix route behind the session requirement, as the console does.
type required struct {
	mux  *http.ServeMux
	flow *auth.Flow
}

func (m required) HandleFunc(p string, h func(http.ResponseWriter, *http.Request)) {
	m.mux.HandleFunc(p, m.flow.Require(h))
}
