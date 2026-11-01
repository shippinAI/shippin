// console serves the Shippin.Cloud console SPA and its read API.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/api"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mail"
	"shippin.cloud/console/internal/spa"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	database, err := db.OpenHolding(ctx, cfg.DatabaseURL, cfg.Migrations.Hold)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	api.ApplyIssuerOverride(ctx, cfg, database) // ops-edited idp.issuer (platform settings)
	store, err := auth.NewStore(database, cfg.SessionKey)
	if err != nil {
		log.Fatalf("sessions: %v", err)
	}
	flow := auth.NewFlow(cfg, store)
	mux := http.NewServeMux()
	consoleAPI := api.New(cfg, flow, database)
	consoleAPI.StartJobs(ctx)
	consoleAPI.SeedTemplates(ctx)
	consoleAPI.SyncStaff(ctx)
	consoleAPI.Register(mux)
	consoleAPI.RegisterGoldSign(mux)
	consoleAPI.RegisterHub(mux)
	consoleAPI.RegisterVaultixHosted(mux)
	go consoleAPI.RunSweeper(ctx)
	startMail(ctx, cfg, database, consoleAPI)
	mux.HandleFunc("GET /auth/login", flow.Login)
	mux.HandleFunc("GET /auth/callback", flow.Callback)
	mux.HandleFunc("GET /auth/logout", flow.Logout)
	mux.Handle("/", byOrigin(flow, spa.Handler(cfg.StaticDir), spa.HandlerFor(cfg.StaticDir, "ops.html")))
	srv := &http.Server{Addr: cfg.Listen, Handler: api.RequestLog(secure(api.SameOrigin(mux))), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("console listening on %s (domain %s)", cfg.Listen, cfg.Domain)
	log.Fatal(srv.ListenAndServe())
}

// byOrigin serves the operator app (its own bundle) on the operator origin, the console elsewhere.
func byOrigin(f *auth.Flow, console, ops http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.IsOperatorOrigin(r) {
			ops.ServeHTTP(w, r)
			return
		}
		console.ServeHTTP(w, r)
	})
}

func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		h.ServeHTTP(w, r)
	})
}

// startMail turns on e-mail notices: the API queues them, the worker submits them.
func startMail(ctx context.Context, cfg *config.Config, d *db.DB, a *api.API) {
	if !cfg.Mail.Enabled {
		log.Printf("mail: disabled; notices are not queued")
		return
	}
	r, err := mail.NewRenderer()
	if err != nil {
		log.Fatalf("mail templates: %v", err)
	}
	t, err := mail.NewSMTP(cfg.Mail)
	if err != nil {
		log.Fatalf("mail: %v", err)
	}
	footer := "You get this because of your role in " + cfg.Brand.Name + ". Sent by " + cfg.Mail.From + "."
	a.SetNotifier(&mail.Notifier{R: r, Q: d, Brand: cfg.Brand.Name, Footer: footer})
	go mail.NewWorker(d, t, cfg.Mail).Run(ctx)
	log.Printf("mail: notices via %s:%d (%s) from %s", cfg.Mail.Host, cfg.Mail.Port, cfg.Mail.TLS, cfg.Mail.From)
}
