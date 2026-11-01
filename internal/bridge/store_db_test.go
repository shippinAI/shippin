package bridge

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"shippin.cloud/console/internal/db"
)

// Needs SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated).
func testStore(t *testing.T) (*Store, context.Context) {
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
	return &Store{Pool: d.Pool}, ctx
}

func TestDeviceFlowBindsAccountAndRevokes(t *testing.T) {
	s, ctx := testStore(t)
	_, dev, code, err := s.Start(ctx, "dabo", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(ctx, dev, "brg_"); !errors.Is(err, ErrPending) {
		t.Fatalf("want pending, got %v", err)
	}
	if name, _ := s.Pending(ctx, NormalizeUserCode(code[:4]+" "+code[5:])); name != "dabo" {
		t.Fatalf("pending lookup %q", name)
	}
	ok, err := s.Decide(ctx, code, true, "sub-1", "j@example.com", map[string]string{"Sub": "sub-1"}, time.Hour)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if again, _ := s.Decide(ctx, code, true, "sub-2", "x@example.com", nil, time.Hour); again {
		t.Fatal("a code was approved twice")
	}
	tok, ses, err := s.Redeem(ctx, dev, "brg_")
	if err != nil || ses.Sub != "sub-1" || ses.Email != "j@example.com" {
		t.Fatal(ses, err)
	}
	if _, _, err := s.Redeem(ctx, dev, "brg_"); !errors.Is(err, ErrExpired) {
		t.Fatal("device code redeemed twice")
	}
	if x, _ := s.ByToken(ctx, tok); x == nil || x.ID != ses.ID {
		t.Fatal("token not accepted")
	}
	if x, _ := s.ByToken(ctx, tok+"x"); x != nil {
		t.Fatal("wrong token accepted")
	}
	if ok, _ := s.Revoke(ctx, "sub-2", ses.ID); ok {
		t.Fatal("another account revoked the session")
	}
	if ok, _ := s.Revoke(ctx, "sub-1", ses.ID); !ok {
		t.Fatal("revoke failed")
	}
	if x, _ := s.ByToken(ctx, tok); x != nil {
		t.Fatal("revoked token still accepted")
	}
	list, _ := s.List(ctx, "sub-1")
	if len(list) != 1 || list[0].Status != "revoked" {
		t.Fatalf("list %+v", list)
	}
}

func TestDeniedCodeNeverIssues(t *testing.T) {
	s, ctx := testStore(t)
	_, dev, code, _ := s.Start(ctx, "x", time.Minute)
	if ok, _ := s.Decide(ctx, code, false, "sub-1", "e", nil, time.Hour); !ok {
		t.Fatal("deny failed")
	}
	if _, _, err := s.Redeem(ctx, dev, "brg_"); !errors.Is(err, ErrDenied) {
		t.Fatalf("got %v", err)
	}
}
