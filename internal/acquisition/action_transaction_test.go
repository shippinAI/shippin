package acquisition_test

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/servers"
	"shippin.cloud/console/internal/tenancy/acquisition_source"
	"strings"
	"testing"
	"time"
)

type pageFetcher struct{ lead acquisition.Lead }

func (f pageFetcher) Fetch(_ context.Context, _ acquisition.SourceConfig, _ []byte, after uint64) (acquisition.Page, error) {
	p := acquisition.Page{Items: []acquisition.Lead{}, Cursor: after, RetentionDays: 30}
	if after == 0 {
		p.Items = []acquisition.Lead{f.lead}
		p.Cursor = f.lead.Revision
	}
	return p, nil
}

// This exercises the real MCPL transaction/audit runner, not its external auth gate.
// Real-session browser acceptance qualifies that distinct boundary.
func TestDisposableActionAuditFailureRollsBackImportedPage(t *testing.T) {
	if os.Getenv("SHIPPIN_TEST_ACQUISITION_DISPOSABLE") != "1" {
		t.Skip("requires disposable acquisition opt-in")
	}
	raw := os.Getenv("SHIPPIN_TEST_ACQUISITION_DSN")
	if raw == "" {
		t.Skip("requires disposable acquisition DSN")
	}
	cfg, e := pgxpool.ParseConfig(raw)
	if e != nil || cfg.ConnConfig.Database != "acquisition_disposable" {
		t.Fatal("wrong disposable database")
	}
	ctx := context.Background()
	admin, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal("test database unavailable")
	}
	defer admin.Close()
	schema := "acq_action_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal("test schema unavailable")
	}
	defer func() {
		if _, e := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); e != nil {
			t.Error("test cleanup failed")
		}
	}()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	_, e = pool.Exec(ctx, `CREATE TABLE staff_roles(key text PRIMARY KEY, permissions text[], updated_at timestamptz); CREATE TABLE audit_probe(action text)`)
	if e != nil {
		t.Fatal(e)
	}
	migration, e := os.ReadFile(filepath.Join("..", "db", "migrations", "073_shippin_acquisition.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, string(migration)); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	sealer, e := servers.NewSealer(strings.Repeat("11", 32))
	if e != nil {
		t.Fatal(e)
	}
	repo := acquisition.PostgresRepository{}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	source, e := repo.Create(ctx, tx, acquisition.CreateCommand{Label: "Synthetic source", Origin: "https://127.0.0.1", DialIP: "127.0.0.1", Port: 18082, CAPEM: ca, SourceTenant: "fixture", SourceProject: "signup", Issuer: "fixture", Audience: "source-admin", SigningSeed: []byte(strings.Repeat("x", 32))}, sealer)
	if e != nil {
		t.Fatal(e)
	}
	_, e = repo.Update(ctx, tx, source.ID, acquisition.UpdateCommand{ExpectedConfigGeneration: source.ConfigGeneration, Label: "Synthetic source", Origin: "https://127.0.0.1", DialIP: "127.0.0.1", Port: 18082, CAPEM: ca, Paused: false}, sealer)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	lead := acquisition.Lead{ID: strings.Repeat("a", 32), Email: "audit-proof@example.test", Status: "new", ConsentVersion: "early-access-v1", CreatedAt: now, ConsentAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour), Revision: 1}
	svcs := mcpl.Services{}
	mcpl.Provide[acquisition.Repository](svcs, repo)
	mcpl.Provide[acquisition.Sealer](svcs, sealer)
	mcpl.Provide[acquisition.Fetcher](svcs, pageFetcher{lead})
	action := mcpl.Register("shippin.tenancy.acquisition_source.sync", acquisition_source.Sync)
	mux := http.NewServeMux()
	failAudit := true
	audits := 0
	mux.HandleFunc(action.Route, func(w http.ResponseWriter, r *http.Request) {
		action.Serve(&mcpl.Call{Ctx: r.Context(), Pool: pool, Actor: "synthetic-staff", Services: svcs, Audit: func(q mcpl.DB, action, detail string) error {
			audits++
			if _, e := q.Exec(r.Context(), "INSERT INTO audit_probe(action) VALUES($1)", action); e != nil {
				return e
			}
			if failAudit {
				return errors.New("synthetic audit unavailable")
			}
			return nil
		}}, w, r)
	})
	call := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/operator/tenancy/acquisition-sources/"+source.ID+"/sync", strings.NewReader(`{"reason":"Isolated audit atomicity qualification"}`))
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("unexpected action status %d", w.Code)
		}
		if strings.Contains(w.Body.String(), lead.Email) || strings.Contains(w.Body.String(), base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))) {
			t.Fatal("sync disclosed personal data or signer")
		}
	}
	counts := func(cursor, leads, audit int) {
		t.Helper()
		var c, l, a int
		if e := pool.QueryRow(ctx, "SELECT cursor,(SELECT count(*) FROM shippin_acquisition_leads),(SELECT count(*) FROM audit_probe) FROM shippin_acquisition_sources WHERE id=$1", source.ID).Scan(&c, &l, &a); e != nil || c != cursor || l != leads || a != audit {
			t.Fatalf("transaction outcome cursor=%d leads=%d audit=%d", c, l, a)
		}
	}
	call(503)
	counts(0, 0, 0)
	if audits != 1 {
		t.Fatal("audit failure was not exercised")
	}
	failAudit = false
	call(200)
	counts(1, 1, 1)
	call(200)
	counts(1, 1, 2)
	saved, e := repo.ListLeads(ctx, pool, source.ID, sealer)
	if e != nil || saved.Total != 1 || len(saved.Items) != 1 || saved.Items[0].Email != lead.Email {
		t.Fatal("committed encrypted page cannot be read")
	}
}
