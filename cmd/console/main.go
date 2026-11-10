// console serves the Shippin.Cloud console SPA and its read API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"shippin.cloud/console/internal/api"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/clickhouse"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/identity"
	"shippin.cloud/console/internal/mail"
	"shippin.cloud/console/internal/spa"
	"shippin.cloud/console/internal/tlsfile"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		runSetup()
		return
	}
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
	directory := identity.New(database.Pool, cfg, mailSender(cfg), captcha(cfg))
	if cfg.RedisURL != "" {
		shared, err := identity.NewSharedLimits(ctx, cfg.RedisURL, cfg.RedisPrefix)
		if err != nil {
			log.Fatal("Dragonfly/Redis authentication limiter is unavailable; check its connection configuration")
		}
		defer shared.Close()
		directory.UseSharedLimits(shared)
		consoleAPI.SetCacheHealth(shared.Ping)
		log.Print("authentication limits: shared Dragonfly/Redis store connected")
	}
	if cfg.ClickHouseURL != "" {
		consoleAPI.SetClickHouse(openClickHouse(ctx, cfg.ClickHouseURL))
	} else {
		log.Print("clickhouse: not configured; metrics history is unavailable (set SHIPPIN_CLICKHOUSE_URL)")
	}
	consoleAPI.SetDirectory(directory)
	consoleAPI.StartJobs(ctx)
	consoleAPI.SeedTemplates(ctx)
	consoleAPI.SyncStaff(ctx)
	consoleAPI.Register(mux)
	consoleAPI.RegisterHub(mux)
	consoleAPI.RegisterVaultixHosted(mux)
	go consoleAPI.RunSweeper(ctx)
	go consoleAPI.RunNetworkInventory(ctx)
	go consoleAPI.RunPlatformHealth(ctx)
	consoleAPI.RecoverOperations(ctx)
	startMail(ctx, cfg, database, consoleAPI)
	mux.HandleFunc("GET /auth/login", flow.Login)
	mux.HandleFunc("GET /auth/callback", flow.Callback)
	mux.HandleFunc("GET /auth/logout", flow.Logout)
	mux.Handle("/", byOrigin(flow, spa.Handler(cfg.StaticDir), spa.HandlerFor(cfg.StaticDir, "ops.html")))
	srv := &http.Server{Addr: cfg.Listen, Handler: api.RequestLog(secure(api.SameOrigin(mux))), ReadHeaderTimeout: 10 * time.Second}
	if certFile, keyFile := os.Getenv("SHIPPIN_TLS_CERT"), os.Getenv("SHIPPIN_TLS_KEY"); certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			log.Fatal("tls: set both SHIPPIN_TLS_CERT and SHIPPIN_TLS_KEY")
		}
		l, err := tlsfile.New(certFile, keyFile)
		if err != nil {
			log.Fatalf("tls: %v", err)
		}
		srv.TLSConfig = l.Config()
		log.Printf("console listening on %s with TLS (domain %s)", cfg.Listen, cfg.Domain)
		log.Fatal(srv.ListenAndServeTLS("", ""))
	}
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

// openClickHouse connects to the metrics and events store and applies its migrations. ClickHouse may still be
// starting, so it retries for a minute; after that the panel runs without history and /healthz says so.
func openClickHouse(ctx context.Context, raw string) *clickhouse.Client {
	c, err := clickhouse.Open(raw)
	if err != nil {
		log.Fatalf("SHIPPIN_CLICKHOUSE_URL: %v", err)
	}
	for attempt := 1; ; attempt++ {
		applied, err := c.Migrate(ctx)
		if err == nil {
			log.Printf("clickhouse: database %s ready, %d migration(s) applied", c.Database(), len(applied))
			return c
		}
		if attempt == 12 {
			log.Printf("clickhouse: %v; continuing without metrics history", err)
			return c
		}
		time.Sleep(5 * time.Second)
	}
}

func runSetup() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	database, err := db.OpenHolding(ctx, cfg.DatabaseURL, cfg.Migrations.Hold)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Pool.Close()
	code, err := identity.New(database.Pool, cfg, nil, nil).IssueSetup(ctx)
	if errors.Is(err, identity.ErrOwnerExists) {
		fmt.Println("setup_code=")
		return
	}
	if err != nil {
		log.Fatalf("setup: %v", err)
	}
	fmt.Println("setup_code=" + code)
}

func mailSender(cfg *config.Config) identity.Sender {
	if path := os.Getenv("SHIPPIN_MAIL_SINK"); path != "" {
		return identity.FileSender{Path: path}
	}
	if !cfg.Mail.Enabled {
		return nil
	}
	t, err := mail.NewSMTP(cfg.Mail)
	if err != nil {
		log.Fatalf("mail: %v", err)
	}
	return smtpSender{t: t, from: cfg.Mail.From}
}

type smtpSender struct {
	t    *mail.SMTP
	from string
}

func (s smtpSender) Send(ctx context.Context, to, subject, body string) error {
	raw := []byte("Subject: " + subject + "\r\nTo: " + to + "\r\n\r\n" + body)
	return s.t.Send(ctx, s.from, to, raw)
}

func captcha(cfg *config.Config) identity.Captcha {
	if cfg.Directory.Captcha.Provider == "turnstile" {
		return identity.Turnstile{Secret: os.Getenv(cfg.Directory.Captcha.SecretEnv)}
	}
	return identity.NoneCaptcha{}
}
