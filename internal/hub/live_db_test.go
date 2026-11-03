package hub

import (
	"context"
	"testing"
	"time"
)

func TestHistoryRecordsEveryStepWithNotes(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	task, _ := s.Create(ctx, "o", "t", "", "j")
	_, _ = s.Claim(ctx, "o", task.ID, "a1", time.Minute)
	_, _ = s.Progress(ctx, "o", task.ID, "a1", "tests green", time.Minute)
	_, _ = s.RequestApproval(ctx, "o", task.ID, "a1", "ready")
	_, _ = s.Decide(ctx, "o", task.ID, "owner", true, "ship it")
	_, _ = s.Finish(ctx, "o", task.ID, "a1")
	es, err := s.Events(ctx, "o", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"created", "claimed", "progress", "approval_requested", "approved", "done"}
	if len(es) != len(want) {
		t.Fatalf("got %d events: %+v", len(es), es)
	}
	for i, k := range want {
		if es[i].Kind != k {
			t.Fatalf("event %d: %q want %q", i, es[i].Kind, k)
		}
	}
	if es[2].Note != "tests green" || es[4].Note != "ship it" || es[4].Actor != "owner" {
		t.Fatalf("notes/actors: %+v", es)
	}
	if _, err := s.Events(ctx, "other", task.ID); err != ErrNotFound {
		t.Fatalf("other org must not read history: %v", err)
	}
}

func TestLiveFansOutOnlyToTheOrg(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := NewLive()
	go l.Run(ctx, s.Pool)
	mine, stop := l.Subscribe("o")
	defer stop()
	theirs, stop2 := l.Subscribe("x")
	defer stop2()
	time.Sleep(300 * time.Millisecond) // LISTEN is up
	task, _ := s.Create(ctx, "o", "t", "", "j")
	select {
	case c := <-mine:
		if c.TaskID != task.ID || c.Kind != "created" {
			t.Fatalf("change: %+v", c)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no live change")
	}
	select {
	case c := <-theirs:
		t.Fatalf("leaked to another org: %+v", c)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestOwnerByVerifiedEmailOnly(t *testing.T) {
	h := &Handler{OwnerEmail: "owner@example.com"}
	if !h.IsOwner(&Principal{Sub: "s", Email: "Owner@Example.com", EmailVerified: true}) {
		t.Fatal("verified e-mail must match")
	}
	if h.IsOwner(&Principal{Sub: "s", Email: "owner@example.com"}) {
		t.Fatal("unverified e-mail must not approve")
	}
}
