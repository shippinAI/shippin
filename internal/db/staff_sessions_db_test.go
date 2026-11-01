package db

import (
	"testing"
	"time"
)

func TestStaffSessionLifecycle(t *testing.T) {
	d, ctx := testDB(t)
	tn, _ := seed(t, d, ctx)
	e := AuditEntry{Actor: "u1", ActorName: "U", Action: "staff.mode_enter", Detail: "ticket 42 support"}
	s, err := d.StartStaffSession(ctx, StaffSession{Actor: "u1", Reason: "ticket 42 support", FromTenant: tn.ID, AuthAt: time.Now()}, "U", time.Hour, e)
	if err != nil || s.FromTenant != tn.ID || time.Until(s.ExpiresAt) < 59*time.Minute {
		t.Fatalf("start: %+v %v", s, err)
	}
	s2, err := d.StartStaffSession(ctx, StaffSession{Actor: "u1", Reason: "second session", AuthAt: time.Now()}, "U", time.Hour, e)
	if err != nil || s2.ID == s.ID {
		t.Fatalf("restart: %v", err)
	}
	act, _ := d.ActiveStaffSession(ctx, "u1")
	if act == nil || act.ID != s2.ID {
		t.Fatalf("active = %+v, want the second session only", act)
	}
	if ok, err := d.EndStaffSession(ctx, "u1", AuditEntry{Actor: "u1", Action: "staff.mode_exit"}); !ok || err != nil {
		t.Fatalf("end: %v %v", ok, err)
	}
	if act, _ := d.ActiveStaffSession(ctx, "u1"); act != nil {
		t.Fatal("session still active after end")
	}
	if ok, _ := d.EndStaffSession(ctx, "u1", AuditEntry{Actor: "u1", Action: "staff.mode_exit"}); ok {
		t.Fatal("second end should find nothing")
	}
	// Expired sessions don't count.
	if _, err := d.StartStaffSession(ctx, StaffSession{Actor: "u2", Reason: "short session", AuthAt: time.Now()}, "U", time.Millisecond, e); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if act, _ := d.ActiveStaffSession(ctx, "u2"); act != nil {
		t.Fatal("expired session reported active")
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action LIKE 'staff.mode_%'`).Scan(&n)
	if n != 4 {
		t.Fatalf("audit rows = %d, want 4 (3 enters + 1 exit)", n)
	}
}
