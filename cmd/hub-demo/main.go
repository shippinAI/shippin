// hub-demo runs the agent hub board locally behind the real console sign-in (auth.Flow: OIDC
// PKCE, encrypted sessions, IdP-validated bearer tokens) against a loopback stand-in for Nomen.
// People sign in in the browser; agents use bearer tokens from HUB_DEMO_TOKENS. Loopback only;
// never deploy it.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hub"
	"shippin.cloud/console/internal/spa"
)

func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	return err == nil && net.ParseIP(host).IsLoopback()
}

func main() {
	addr, idpAddr, url := os.Getenv("HUB_DEMO_ADDR"), os.Getenv("HUB_DEMO_IDP_ADDR"), os.Getenv("DATABASE_URL")
	if !loopback(addr) || !loopback(idpAddr) || url == "" {
		log.Fatal("set HUB_DEMO_ADDR and HUB_DEMO_IDP_ADDR to loopback host:port, and DATABASE_URL")
	}
	idp, err := loadIdP(os.Getenv("HUB_DEMO_USERS"), os.Getenv("HUB_DEMO_TOKENS"))
	if err != nil {
		log.Fatalf("idp: %v", err)
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
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
	if raw, err := os.ReadFile(os.Getenv("SHIPPIN_CONFIG")); err != nil || json.Unmarshal(raw, cfg) != nil {
		log.Fatal("set SHIPPIN_CONFIG to the console config (brand and IdP name come from it)")
	}
	cfg.ConsoleHost, cfg.Operator = addr, config.Operator{}
	cfg.IDP = config.IDP{Name: cfg.IDP.Name, Issuer: "http://" + idpAddr, ClientID: "hub-demo"}
	flow := auth.NewFlow(cfg, store)
	who := func(w http.ResponseWriter, r *http.Request) (*hub.Principal, bool) {
		id, err := flow.Whoami(r.Context(), auth.FromContext(r.Context()))
		if err != nil {
			auth.Problem(w, http.StatusUnauthorized, "Your session expired. Sign in again.")
			return nil, false
		}
		return &hub.Principal{Sub: id.Sub, Name: id.Name, OrgID: id.OrgID, Email: id.Email, EmailVerified: id.EmailVerified}, true
	}
	live := hub.NewLive()
	go live.Run(ctx, d.Pool)
	mux := http.NewServeMux()
	h := &hub.Handler{Store: &hub.Store{Pool: d.Pool}, Who: who, OwnerSub: os.Getenv("HUB_OWNER_SUB"),
		OwnerEmail: os.Getenv("HUB_OWNER_EMAIL"), Lease: leaseFromEnv(), Live: live}
	h.Register(required{mux, flow})
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
		_ = json.NewEncoder(w).Encode(map[string]any{"brand": cfg.Brand, "domain": cfg.Domain, "idp_name": cfg.IDP.Name, "idp_host": cfg.IDP.Issuer,
			"login_ready": true, "app": "console", "console_url": "https://" + addr})
	})
	mux.Handle("/", spa.Handler(os.Getenv("HUB_DEMO_STATIC")))
	imux := http.NewServeMux()
	idp.register(imux)
	go func() { log.Fatal(http.ListenAndServe(idpAddr, imux)) }()
	log.Printf("hub-demo on https://%s (IdP stand-in on http://%s)", addr, idpAddr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second, TLSConfig: selfSigned()}
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

func leaseFromEnv() time.Duration {
	if v, err := time.ParseDuration(os.Getenv("HUB_LEASE")); err == nil && v > 0 {
		return v
	}
	return 10 * time.Minute
}

// required puts every hub route behind the session/bearer requirement, as the console does.
type required struct {
	mux  *http.ServeMux
	flow *auth.Flow
}

func (m required) HandleFunc(p string, h func(http.ResponseWriter, *http.Request)) {
	m.mux.HandleFunc(p, m.flow.Require(h))
}
