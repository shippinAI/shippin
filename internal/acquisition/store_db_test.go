package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/servers"
)

type fetchFunc func(context.Context, SourceConfig, []byte, uint64) (Page, error)

func (f fetchFunc) Fetch(c context.Context, s SourceConfig, k []byte, a uint64) (Page, error) {
	return f(c, s, k, a)
}

type failingSealer struct {
	Sealer
	remaining int
}

func (s *failingSealer) Seal(b []byte, aad string) ([]byte, error) {
	s.remaining--
	if s.remaining == 0 {
		return nil, errors.New("synthetic seal failure")
	}
	return s.Sealer.Seal(b, aad)
}

func disposablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("SHIPPIN_TEST_ACQUISITION_DISPOSABLE") != "1" {
		t.Skip("requires explicit disposable acquisition database opt-in")
	}
	dsn := os.Getenv("SHIPPIN_TEST_ACQUISITION_DSN")
	if dsn == "" {
		t.Skip("requires SHIPPIN_TEST_ACQUISITION_DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || cfg.ConnConfig.Database != "acquisition_disposable" {
		t.Fatal("test database must be acquisition_disposable")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	var database string
	if err = admin.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil || database != "acquisition_disposable" {
		admin.Close()
		t.Fatal("wrong disposable database")
	}
	schema := "acq_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal("test schema unavailable")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Close()
		t.Fatal("test pool unavailable")
	}
	t.Cleanup(func() {
		pool.Close()
		_, e := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
		if e != nil {
			t.Error("test schema cleanup failed")
		}
	})
	_, err = pool.Exec(ctx, `CREATE TABLE staff_roles(key text PRIMARY KEY,name text,description text,permissions text[],preset boolean,updated_at timestamptz DEFAULT now()); INSERT INTO staff_roles(key,permissions) VALUES('staff.owner',ARRAY['existing.permission']);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "db", "migrations", "073_shippin_acquisition.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	return pool
}
func dbtx(t *testing.T, p *pgxpool.Pool, fn func(pgx.Tx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	e = fn(tx)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func testLead(id string, rev uint64, now time.Time) Lead {
	return Lead{ID: id, Revision: rev, Email: "synthetic@example.test", Status: "new", ConsentVersion: "early-access-v1", CreatedAt: now, ConsentAt: now, UpdatedAt: now, ExpiresAt: now.AddDate(0, 0, 30)}
}

func TestDisposableStoreAtomicityDeletionExpiryAndFencing(t *testing.T) {
	pool := disposablePool(t)
	ctx := context.Background()
	repo := PostgresRepository{}
	seal, err := servers.NewSealer(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	origin, ip, port, ca := tlsSource(t, nil)
	input := CreateCommand{Label: "Synthetic", Origin: origin, DialIP: ip, Port: port, CAPEM: ca, SourceTenant: "tenant", SourceProject: "project", Issuer: "issuer", Audience: "audience", SigningSeed: make([]byte, 32)}
	var summary SourceSummary
	if err = dbtx(t, pool, func(tx pgx.Tx) error { var e error; summary, e = repo.Create(ctx, tx, input, seal); return e }); err != nil {
		t.Fatal(err)
	}
	id := summary.ID
	var ownerPerms []string
	if err = pool.QueryRow(ctx, `SELECT permissions FROM staff_roles WHERE key='staff.owner'`).Scan(&ownerPerms); err != nil || !containsTest(ownerPerms, "existing.permission") || !containsTest(ownerPerms, "acquisition.read") || !containsTest(ownerPerms, "acquisition.sync") {
		t.Fatal("owner permissions lost or missing")
	}
	var stored []byte
	if err = pool.QueryRow(ctx, `SELECT sealed_signer FROM shippin_acquisition_sources WHERE id=$1`, id).Scan(&stored); err != nil || string(stored) == string(input.SigningSeed) {
		t.Fatal("signer not sealed")
	}
	detail, e := repo.Get(ctx, pool, id)
	if e != nil || detail.SourceProject != "project" || !detail.Summary.Paused {
		t.Fatal("wrong source")
	}
	upd := UpdateCommand{ExpectedConfigGeneration: summary.ConfigGeneration, Label: input.Label, Origin: input.Origin, DialIP: input.DialIP, Port: input.Port, CAPEM: input.CAPEM, Paused: false}
	if err = dbtx(t, pool, func(tx pgx.Tx) error { var e error; summary, e = repo.Update(ctx, tx, id, upd, seal); return e }); err != nil {
		t.Fatal(err)
	}
	// Exercise a valid source RFC3339Nano timestamp that PostgreSQL cannot store exactly.
	now := time.Now().UTC().Truncate(time.Microsecond).Add(789 * time.Nanosecond)
	lead := testLead("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, now)
	page := Page{Items: []Lead{lead}, Cursor: 1, RetentionDays: 30}
	fetch := func(p Page) fetchFunc {
		return func(_ context.Context, _ SourceConfig, _ []byte, _ uint64) (Page, error) { return p, nil }
	}
	sync := func(p Page) error {
		return dbtx(t, pool, func(tx pgx.Tx) error { _, e := repo.Sync(ctx, tx, id, fetch(p), seal, now); return e })
	}
	if err = sync(page); err != nil {
		t.Fatal(err)
	}
	preview, e := repo.ListLeads(ctx, pool, id, seal)
	if e != nil || preview.Total != 1 || preview.Items[0].Email != lead.Email || preview.Items[0].EmailVerified {
		t.Fatal("import preview failed")
	}
	if !preview.Items[0].ExpiresAt.Equal(lead.ExpiresAt.Truncate(time.Microsecond)) || preview.Items[0].ExpiresAt.After(lead.ExpiresAt) {
		t.Fatal("source expiry was not safely canonicalized to database precision")
	}
	if err = pool.QueryRow(ctx, `SELECT sealed_payload FROM shippin_acquisition_leads WHERE source_id=$1 AND lead_id=$2`, id, lead.ID).Scan(&stored); err != nil || strings.Contains(string(stored), lead.Email) {
		t.Fatal("lead not sealed")
	}
	// A page failing after its first applied record must be rolled back with cursor.
	two := testLead("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 2, now)
	three := testLead("cccccccccccccccccccccccccccccccc", 3, now)
	bad := Page{Items: []Lead{two, three}, Cursor: 3, RetentionDays: 30}
	fail := &failingSealer{Sealer: seal, remaining: 2}
	err = dbtx(t, pool, func(tx pgx.Tx) error { _, e := repo.Sync(ctx, tx, id, fetch(bad), fail, now); return e })
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("seal failure: %v", err)
	}
	detail, e = repo.Get(ctx, pool, id)
	if e != nil || detail.Summary.Cursor != 1 || detail.Summary.ActiveLeadCount != 1 {
		t.Fatal("partial transaction committed")
	}
	// Deletion leaves a permanent terminal floor even against a newer live record.
	if err = sync(Page{Items: []Lead{{ID: lead.ID, Revision: 2, Deleted: true}}, Cursor: 2, RetentionDays: 30}); err != nil {
		t.Fatal(err)
	}
	lead.Revision = 3
	if err = sync(Page{Items: []Lead{lead}, Cursor: 3, RetentionDays: 30}); err != nil {
		t.Fatal(err)
	}
	var deleted bool
	var revision int64
	if err = pool.QueryRow(ctx, `SELECT deleted,revision,sealed_payload FROM shippin_acquisition_leads WHERE source_id=$1 AND lead_id=$2`, id, lead.ID).Scan(&deleted, &revision, &stored); err != nil || !deleted || revision != 2 || stored != nil {
		t.Fatal("terminal tombstone resurrected")
	}
	// A live expired row is purged, an expired replay is not reintroduced, and new consent can renew it.
	old := testLead("dddddddddddddddddddddddddddddddd", 4, now.AddDate(0, 0, -29))
	if err = sync(Page{Items: []Lead{old}, Cursor: 4, RetentionDays: 30}); err != nil {
		t.Fatal(err)
	}
	more, e := repo.PurgeExpired(ctx, pool, now.AddDate(0, 0, 2), 1)
	if e != nil || !more {
		t.Fatal("purge failed")
	}
	old.Revision = 5
	if err = sync(Page{Items: []Lead{old}, Cursor: 5, RetentionDays: 30}); err != nil {
		t.Fatal(err)
	}
	// At the simulated later clock it is expired; do not insert it via replay.
	err = dbtx(t, pool, func(tx pgx.Tx) error {
		_, e := repo.Sync(ctx, tx, id, fetch(Page{Items: []Lead{func() Lead { x := old; x.Revision = 6; return x }()}, Cursor: 6, RetentionDays: 30}), seal, now.AddDate(0, 0, 2))
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shippin_acquisition_leads WHERE source_id=$1 AND lead_id=$2`, id, old.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired replay retained")
	}
	fresh := testLead(old.ID, 7, now)
	if err = sync(Page{Items: []Lead{fresh}, Cursor: 7, RetentionDays: 30}); err != nil {
		t.Fatal(err)
	}
	// Pause/config update while fetch is blocked fences stale in-flight work.
	started := make(chan struct{})
	resume := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		tx, e := pool.Begin(ctx)
		if e != nil {
			result <- e
			return
		}
		defer tx.Rollback(ctx)
		f := fetchFunc(func(context.Context, SourceConfig, []byte, uint64) (Page, error) {
			close(started)
			<-resume
			return Page{Items: []Lead{}, Cursor: 7, RetentionDays: 30}, nil
		})
		_, e = repo.Sync(ctx, tx, id, f, seal, now)
		result <- e
	}()
	<-started
	detail, e = repo.Get(ctx, pool, id)
	if e != nil {
		t.Fatal(e)
	}
	upd.ExpectedConfigGeneration = detail.Summary.ConfigGeneration
	upd.Paused = true
	if err = dbtx(t, pool, func(tx pgx.Tx) error { _, e := repo.Update(ctx, tx, id, upd, seal); return e }); err != nil {
		t.Fatal(err)
	}
	close(resume)
	if e = <-result; !errors.Is(e, ErrConflict) {
		t.Fatalf("stale pull not fenced: %v", e)
	}
	detail, e = repo.Get(ctx, pool, id)
	if e != nil || !detail.Summary.Paused || detail.Summary.Cursor != 7 {
		t.Fatal("stale cursor changed")
	}
	// Expired rows are not disclosed even if retention cleanup has not run.
	expired := testLead("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", 8, now.AddDate(0, 0, -31))
	raw, err := json.Marshal(expired)
	if err != nil {
		t.Fatal(err)
	}
	box, err := seal.Seal(raw, leadAAD(id, expired.ID, expired.Revision))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO shippin_acquisition_leads(source_id,lead_id,revision,expires_at,sealed_payload) VALUES($1,$2,$3,$4,$5)`, id, expired.ID, expired.Revision, expired.ExpiresAt, box); err != nil {
		t.Fatal(err)
	}
	visible, e := repo.ListLeads(ctx, pool, id, seal)
	if e != nil || visible.Total != 1 || len(visible.Items) != 1 || visible.Items[0].ID != fresh.ID {
		t.Fatal("expired payload leaked through list")
	}
	if _, e = repo.PurgeExpired(ctx, pool, now, 10); e != nil {
		t.Fatal(e)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shippin_acquisition_leads WHERE source_id=$1 AND lead_id=$2`, id, expired.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired payload not physically purged")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shippin_acquisition_leads WHERE source_id=$1 AND lead_id=$2 AND deleted`, id, lead.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("retention removed terminal tombstone")
	}
	// Binding ciphertext to source/id/revision fails closed if a stored row is tampered with.
	if _, err = pool.Exec(ctx, `UPDATE shippin_acquisition_leads SET sealed_payload=$3 WHERE source_id=$1 AND lead_id=$2`, id, fresh.ID, box); err != nil {
		t.Fatal(err)
	}
	if _, e = repo.ListLeads(ctx, pool, id, seal); !errors.Is(e, ErrUnavailable) {
		t.Fatal("ciphertext substitution did not fail closed")
	}
	if _, e = repo.Create(ctx, pool, input, seal); !errors.Is(e, ErrUnavailable) {
		t.Fatal("write without transaction accepted")
	}
}
func containsTest(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

// afterFirstScanDB forces a concurrent update at the exact boundary between
// database reads. The old two-SELECT Get combined old config with the new
// generation, whereas a single statement must return a coherent old snapshot.
type afterFirstScanDB struct {
	mcpl.DB
	after func()
}
type afterScanRow struct {
	pgx.Row
	after func()
}

func (r afterScanRow) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	if r.after != nil {
		r.after()
	}
	return err
}
func (d *afterFirstScanDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := d.DB.QueryRow(ctx, sql, args...)
	hook := d.after
	d.after = nil
	return afterScanRow{Row: row, after: hook}
}
func TestDisposableGetReturnsOneConsistentConfigSnapshot(t *testing.T) {
	pool := disposablePool(t)
	ctx := context.Background()
	repo := PostgresRepository{}
	seal, err := servers.NewSealer(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	origin, ip, port, ca := tlsSource(t, nil)
	input := CreateCommand{Label: "Old", Origin: origin, DialIP: ip, Port: port, CAPEM: ca, SourceTenant: "tenant", SourceProject: "project", Issuer: "issuer", Audience: "audience", SigningSeed: make([]byte, 32)}
	var initial SourceSummary
	if err = dbtx(t, pool, func(tx pgx.Tx) error { var e error; initial, e = repo.Create(ctx, tx, input, seal); return e }); err != nil {
		t.Fatal(err)
	}
	updated := UpdateCommand{ExpectedConfigGeneration: initial.ConfigGeneration, Label: "New", Origin: input.Origin, DialIP: input.DialIP, Port: input.Port, CAPEM: input.CAPEM, Paused: false}
	var updateErr error
	wrapped := &afterFirstScanDB{DB: pool, after: func() {
		updateErr = dbtx(t, pool, func(tx pgx.Tx) error { _, e := repo.Update(ctx, tx, initial.ID, updated, seal); return e })
	}}
	got, err := repo.Get(ctx, wrapped, initial.ID)
	if err != nil || updateErr != nil {
		t.Fatalf("get or concurrent update failed: %v %v", err, updateErr)
	}
	if got.Summary.Label != "Old" || got.Summary.ConfigGeneration != initial.ConfigGeneration || !got.Summary.Paused {
		t.Fatalf("mixed source snapshot: %+v", got.Summary)
	}
	current, err := repo.Get(ctx, pool, initial.ID)
	if err != nil || current.Summary.Label != "New" || current.Summary.ConfigGeneration != initial.ConfigGeneration+1 || current.Summary.Paused {
		t.Fatal("concurrent update not observed")
	}
	stale := UpdateCommand{ExpectedConfigGeneration: got.Summary.ConfigGeneration, Label: got.Summary.Label, Origin: got.Origin, DialIP: got.DialIP, Port: got.Port, CAPEM: got.CAPEM, Paused: got.Summary.Paused}
	if e := dbtx(t, pool, func(tx pgx.Tx) error { _, e := repo.Update(ctx, tx, initial.ID, stale, seal); return e }); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale form not fenced: %v", e)
	}
}
