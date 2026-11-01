package api

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mail"
)

// notifyAPI is an API on a disposable Postgres (SHIPPIN_TEST_DATABASE_URL) with fixed
// recipients and synchronous notices.
func notifyAPI(t *testing.T) (*API, *db.DB, db.Tenant) {
	t.Helper()
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `TRUNCATE mail_outbox`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	id := func() string { u, _ := uuid.NewV7(); return u.String() }
	tn := db.Tenant{ID: id(), IdpOrgID: id(), Name: "Acme", Plan: "p"}
	if err := d.CreateTenant(ctx, tn); err != nil {
		t.Fatal(err)
	}
	r, _ := mail.NewRenderer()
	a := &API{cfg: testCfg(), db: d, notifySync: true,
		notify:         &mail.Notifier{R: r, Q: d, Brand: "Shippin", Footer: "f"},
		lookupManagers: func(context.Context, string) ([]string, error) { return []string{"owner@acme.test"}, nil },
		lookupUser: func(_ context.Context, sub string) (string, error) {
			if sub == "machine" {
				return "", nil
			}
			return "admin@shippin.test", nil
		}}
	a.cfg.ConsoleHost = "console.test"
	return a, d, tn
}

func outbox(t *testing.T, d *db.DB) map[string]time.Time {
	rows, err := d.Pool.Query(context.Background(), `SELECT kind || ' ' || to_addr, send_after FROM mail_outbox`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var k string
		var at time.Time
		_ = rows.Scan(&k, &at)
		out[k] = at
	}
	return out
}

func TestNoticesQueuedPerEvent(t *testing.T) {
	a, d, tn := notifyAPI(t)
	ctx := context.Background()
	now := time.Now()
	req := db.Grant{ID: uuid.NewString(), TenantID: tn.ID, Kind: "request", Requester: "1", RequesterName: "Ops",
		Reason: "debug the build", DurationS: 3600, Status: "pending", NoticeAt: &now}
	if err := d.CreateGrant(ctx, req); err != nil {
		t.Fatal(err)
	}
	a.notifyRequested(req.ID)
	a.notifyDecided(req.ID, "approve", "Owner")
	a.notifyDecided(req.ID, "approve", "Owner") // repeated trigger: deduped
	later := now.Add(720 * time.Hour)
	bg := db.Grant{ID: uuid.NewString(), TenantID: tn.ID, Kind: "breakglass", Requester: "1", RequesterName: "Ops",
		Reason: "court order", Category: "legal_hold", DurationS: 3600, Status: "approved", NoticeAt: &later}
	if err := d.CreateGrant(ctx, bg); err != nil {
		t.Fatal(err)
	}
	a.notifyBreakGlass(bg.ID)
	a.notifyWorkspaceReady(tn.Name, &db.Workspace{ID: uuid.NewString(), TenantID: tn.ID, Name: "api-ws", Host: "api-ws.example.test", CreatedBy: "machine"})
	got := outbox(t, d)
	if _, ok := got["workspace_ready owner@acme.test"]; !ok {
		t.Error("a workspace created by a machine user is announced to the managers")
	}
	if len(got) != 4 {
		t.Fatalf("queued %v", got)
	}
	if _, ok := got["access_request owner@acme.test"]; !ok {
		t.Error("request notice must go to the tenant's managers")
	}
	if _, ok := got["access_approved admin@shippin.test"]; !ok {
		t.Error("approval notice must go to the requesting admin")
	}
	if at := got["breakglass owner@acme.test"]; at.Sub(later).Abs() > time.Second {
		t.Fatalf("legal-hold notice must wait for notice_at: %v vs %v", at, later)
	}
}
