package subs

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

// Needs SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated).
func testStore(t *testing.T) (*Store, context.Context, string, string) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	d.Pool.Close()
	if d, err = db.Open(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	tid := uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'operator','founder')`, tid, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	acct, err := d.UpsertAccount(ctx, "sub-"+uuid.NewString(), "j@example.test")
	if err != nil {
		t.Fatal(err)
	}
	return &Store{Pool: d.Pool}, ctx, tid, acct
}

func TestHomeAndConnections(t *testing.T) {
	s, ctx, tid, acct := testStore(t)
	h, err := s.EnsureHome(ctx, tid, acct, "", UtilityLabel(acct)+".ws.test")
	if err != nil || h.Volume != "ai-"+acct || h.State != "creating" || h.UtilityState != "stopped" {
		t.Fatalf("home: %+v %v", h, err)
	}
	again, _ := s.EnsureHome(ctx, tid, acct, "", "ignored")
	if again.ID != h.ID {
		t.Fatal("one AI home per user per placement")
	}
	if got, _ := s.HomeByHost(ctx, h.UtilityHost); got == nil || got.AccountID != acct {
		t.Fatal("home by utility host")
	}
	if _, err := s.HomeByHost(ctx, "nobody.ws.test"); err != ErrNotFound {
		t.Fatal("unknown host")
	}
	if err := s.SetStatus(ctx, h, "claude", Activating, "", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus(ctx, h, "claude", Connected, "", true); err != nil {
		t.Fatal(err)
	}
	cs, _ := s.Connections(ctx, acct)
	if len(cs) != 1 || cs[0].Status != Connected || cs[0].CheckedAt == nil {
		t.Fatalf("connections: %+v", cs)
	}
	if err := s.SetUtility(ctx, h.ID, "running", true); err != nil {
		t.Fatal(err)
	}
	if idle, _ := s.IdleUtilities(ctx, time.Hour); len(idle) != 0 {
		t.Fatal("just touched")
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE ai_homes SET utility_seen=now()-interval '2 hours'`)
	if idle, _ := s.IdleUtilities(ctx, time.Hour); len(idle) != 1 {
		t.Fatal("idle utility")
	}
}

func TestDatabaseRefusesTokens(t *testing.T) {
	s, ctx, tid, acct := testStore(t)
	h, _ := s.EnsureHome(ctx, tid, acct, "", "")
	_ = s.SetStatus(ctx, h, "codex", Connected, "", true)
	_, err := s.Pool.Exec(ctx, `UPDATE subscription_connections SET token_ref='vault:x'`)
	if err == nil || !strings.Contains(err.Error(), "subscription_no_token") {
		t.Fatalf("token_ref must be refused: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO ai_homes(id,tenant_id,account_id,volume) VALUES($1,$2,$3,'ai-../x')`,
		uuid.NewString(), tid, acct); err == nil {
		t.Fatal("volume name must be ai-<uuid>")
	}
	if err := s.SetStatus(ctx, h, "Bad Provider", Connected, "", true); err == nil {
		t.Fatal("provider id check")
	}
}
