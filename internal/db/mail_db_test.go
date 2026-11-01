package db

import (
	"testing"
	"time"
)

func TestMailOutboxClaimRetryDedupe(t *testing.T) {
	d, ctx := testDB(t)
	m := OutMail{Kind: "k", DedupeKey: "k|1|a@x.test", To: "a@x.test", Subject: "s", Text: "t", HTML: "h"}
	if ok, err := d.EnqueueMail(ctx, m); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, _ := d.EnqueueMail(ctx, m); ok {
		t.Fatal("dedupe key must prevent a second row")
	}
	later := m
	later.DedupeKey, later.SendAfter = "k|2|a@x.test", time.Now().Add(time.Hour)
	_, _ = d.EnqueueMail(ctx, later)

	got, err := d.ClaimMail(ctx, 10, time.Minute)
	if err != nil || len(got) != 1 || got[0].DedupeKey != m.DedupeKey {
		t.Fatalf("claim: %v %v (a delayed mail must not be claimed early)", got, err)
	}
	if again, _ := d.ClaimMail(ctx, 10, time.Minute); len(again) != 0 {
		t.Fatal("a leased mail must not be claimed twice")
	}
	if err := d.MailFailed(ctx, got[0].ID, "451", time.Now().Add(-time.Second), false); err != nil {
		t.Fatal(err)
	}
	re, _ := d.ClaimMail(ctx, 10, time.Minute)
	if len(re) != 1 || re[0].Attempts != 1 {
		t.Fatalf("retry claim: %+v", re)
	}
	if err := d.MailSent(ctx, re[0].ID); err != nil {
		t.Fatal(err)
	}
	c, _ := d.MailCounts(ctx)
	if c["sent"] != 1 || c["pending"] != 1 || c["failed"] != 0 {
		t.Fatalf("counts: %v", c)
	}
}
