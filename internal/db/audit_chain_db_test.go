package db

import (
	"fmt"
	"sync"
	"testing"
)

func chainOf(t *testing.T, d *DB) AuditChain {
	t.Helper()
	c, err := d.VerifyAuditChain(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Rows from before the chain are linked once in id order, new rows (also concurrent ones) extend
// it gaplessly, the app can't forge links, and a superuser edit is detected.
func TestAuditHashChain(t *testing.T) {
	pre := func(d *DB) {
		if c := chainOf(t, d); c.Enabled {
			t.Fatal("chain must not exist before the superuser path runs")
		}
		for i := 0; i < 3; i++ {
			if err := d.AddAudit(t.Context(), AuditEntry{Actor: "old", Action: fmt.Sprint("old.", i)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	admin, app := asApp(t, pre)
	ctx := t.Context()
	if c := chainOf(t, app); !c.Enabled || !c.OK || c.Rows != 3 || len(c.Head) != 64 {
		t.Fatalf("after backfill: %+v", c)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := app.AddAudit(ctx, AuditEntry{Actor: "app", Action: "x", Detail: fmt.Sprint(i)}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_, err := app.Pool.Exec(ctx, `INSERT INTO audit_log(actor, action, chain_seq, prev_hash, hash) VALUES('app','forge',1,'\x00','\x00')`)
	if err != nil {
		t.Fatal("insert as app:", err)
	}
	c := chainOf(t, app)
	if !c.OK || c.Rows != 24 {
		t.Fatalf("after app inserts: %+v", c)
	}
	var maxSeq int64
	if err := app.Pool.QueryRow(ctx, `SELECT max(chain_seq) FROM audit_log`).Scan(&maxSeq); err != nil || maxSeq != 24 {
		t.Fatalf("chain_seq must be gapless: %d %v", maxSeq, err)
	}
	for _, q := range []string{`UPDATE audit_log SET hash=NULL`, `ALTER TABLE audit_log DISABLE TRIGGER audit_log_chain`,
		`DROP TRIGGER audit_log_chain ON audit_log`} {
		if _, err := app.Pool.Exec(ctx, q); err == nil {
			t.Errorf("app role must not be able to: %s", q)
		}
	}
	if _, err := admin.Pool.Exec(ctx, `ALTER TABLE audit_log DISABLE TRIGGER audit_log_append_only;
		UPDATE audit_log SET detail='tampered' WHERE chain_seq=7;
		ALTER TABLE audit_log ENABLE TRIGGER audit_log_append_only`); err != nil {
		t.Fatal(err)
	}
	if c := chainOf(t, app); c.OK || c.FirstBad != 7 {
		t.Fatalf("tampering must be detected at 7: %+v", c)
	}
	if err := HardenAudit(ctx, admin, appRole); err != nil { // re-running never re-links rows
		t.Fatal(err)
	}
	if c := chainOf(t, app); c.OK {
		t.Fatal("re-running the superuser path must not hide tampering")
	}
}
