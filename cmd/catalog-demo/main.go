// catalog-demo serves the admin SPA and the customer catalog API against a local database
// with a fixed demo identity. It exists for screenshots and local review only: it refuses to
// listen on anything but loopback, and it never talks to the IdP or hostd.
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
	"shippin.cloud/console/internal/spa"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	listen := env("CATALOG_DEMO_LISTEN", "127.0.0.1:8097")
	host, _, err := net.SplitHostPort(listen)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		log.Fatalf("catalog-demo only listens on loopback, not %q", listen)
	}
	ctx := context.Background()
	d, err := db.Open(ctx, os.Getenv("CATALOG_DEMO_DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if seed := os.Getenv("CATALOG_DEMO_SEED"); seed != "" {
		sql, err := os.ReadFile(seed)
		if err != nil {
			log.Fatalf("seed: %v", err)
		}
		if _, err := d.Pool.Exec(ctx, string(sql)); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}
	identity, err := os.ReadFile(env("CATALOG_DEMO_IDENTITY", "config/demo/identity.json"))
	if err != nil {
		log.Fatalf("identity: %v", err)
	}
	mux := http.NewServeMux()
	h := &catalog.Handler{S: &catalog.Store{Pool: d.Pool}}
	for _, rt := range h.Routes() {
		fn := rt.H
		mux.HandleFunc(rt.Pattern, func(w http.ResponseWriter, r *http.Request) { fn(w, r, "demo") })
	}
	mountIdentity(mux, identity)
	if bin := os.Getenv("CATALOG_DEMO_ENGINE"); bin != "" {
		cfg, err := hostd.LoadConfig(env("CATALOG_DEMO_HOSTD", "deploy/hostd.example.json"))
		if err != nil || cfg.Plans["founder"].VM == nil {
			log.Fatalf("hostd config: %v (needs plans.founder.vm)", err)
		}
		eng := &engines{bin: bin, root: env("CATALOG_DEMO_ENGINE_ROOT", ".run/devspace"), procs: map[string]*exec.Cmd{}}
		mountDevspace(mux, d, identity, eng, cfg.Plans["founder"].VM)
	}
	mux.Handle("/", spa.Handler(env("CATALOG_DEMO_STATIC", "web/dist")))
	log.Printf("catalog-demo on http://%s/ (admin: /admin/customers)", listen)
	srv := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
