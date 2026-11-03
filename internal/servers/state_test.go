package servers

import (
	"testing"
	"time"
)

// TestStateMachine: the transition table and stale after exactly Missed silent intervals.
func TestStateMachine(t *testing.T) {
	type row struct{ issue, beat, revoke bool }
	want := map[State]row{
		Planned: {false, false, false},
		Pending: {true, false, true},
		Active:  {true, true, true},
		Revoked: {false, false, false},
	}
	for st, w := range want {
		if (CanIssue(st) == nil) != w.issue || (CanEnroll(st) == nil) != w.issue {
			t.Errorf("%s issue/enroll: want %v", st, w.issue)
		}
		if (CanBeat(st) == nil) != w.beat {
			t.Errorf("%s beat: want %v", st, w.beat)
		}
		if (CanRevoke(st) == nil) != w.revoke {
			t.Errorf("%s revoke: want %v", st, w.revoke)
		}
	}
	c := Clock{Interval: 15 * time.Second, Missed: 3}
	now := time.Now()
	at := func(ago time.Duration) *time.Time { x := now.Add(-ago); return &x }
	cases := []struct {
		st   State
		seen *time.Time
		want State
	}{
		{Active, at(0), Active},
		{Active, at(44 * time.Second), Active},
		{Active, at(45 * time.Second), Active},
		{Active, at(46 * time.Second), Stale},
		{Active, nil, Stale},
		{Pending, nil, Pending},
		{Planned, nil, Planned},
		{Revoked, at(0), Revoked},
	}
	for _, k := range cases {
		if got := c.Effective(k.st, k.seen, now); got != k.want {
			t.Errorf("%s seen %v: %s, want %s", k.st, k.seen, got, k.want)
		}
	}
}

func TestSecret(t *testing.T) {
	p, h, err := newSecret(TokenPrefix)
	if err != nil || !wellFormed(p, TokenPrefix) || Hash(p) != h || len(h) != 64 {
		t.Fatalf("secret %d %d %v", len(p), len(h), err)
	}
	q, _, _ := newSecret(TokenPrefix)
	if p == q || wellFormed(p, CredPrefix) || wellFormed("shpe_short", TokenPrefix) {
		t.Fatal("secrets must be unique and prefix-checked")
	}
}
