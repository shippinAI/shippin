package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

var fast = Settings{Workers: 2, Lease: 2 * time.Second, Heartbeat: 200 * time.Millisecond,
	Poll: 50 * time.Millisecond, BackoffBase: 10 * time.Millisecond, BackoffMax: 20 * time.Millisecond}

func waitDone(t *testing.T, s *Store, id string) *Job {
	t.Helper()
	for range 200 {
		if j, _ := s.Get(context.Background(), id); j != nil && j.Done() {
			return j
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("job %s never finished", id)
	return nil
}

func TestRunnerRetriesThenSucceeds(t *testing.T) {
	s, _, tid := testJobs(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	r := NewRunner(s, fast)
	r.Register(KindWorkspaceProvision, func(ctx context.Context, j *Job, step func(string)) (any, error) {
		step("creating")
		if calls.Add(1) == 1 {
			return nil, errors.New("host busy")
		}
		return map[string]string{"ip": "10.1.0.5"}, nil
	})
	go r.Run(ctx)
	j, _, _ := s.Enqueue(ctx, job(tid, "w"))
	r.Wake()
	got := waitDone(t, s, j.ID)
	if got.Status != Succeeded || got.Attempts != 2 || got.Step != "creating" || string(got.Result) != `{"ip": "10.1.0.5"}` {
		t.Fatalf("got %+v result=%s", got, got.Result)
	}
}

func TestRunnerPermanentFailsAtOnce(t *testing.T) {
	s, _, tid := testJobs(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewRunner(s, fast)
	r.Register(KindWorkspaceProvision, func(context.Context, *Job, func(string)) (any, error) {
		return nil, Permanent(errors.New("bad template"))
	})
	go r.Run(ctx)
	j, _, _ := s.Enqueue(ctx, job(tid, "w"))
	if got := waitDone(t, s, j.ID); got.Status != Failed || got.Attempts != 1 || got.Error != "bad template" {
		t.Fatalf("got %+v", got)
	}
}

// A console stopping mid-job hands the job back; the next console finishes it.
func TestRunnerResumesAfterRestart(t *testing.T) {
	s, _, tid := testJobs(t)
	started := make(chan struct{})
	ctx1, stop1 := context.WithCancel(context.Background())
	r1 := NewRunner(s, fast)
	r1.Register(KindWorkspaceProvision, func(ctx context.Context, _ *Job, _ func(string)) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	done1 := make(chan struct{})
	go func() { r1.Run(ctx1); close(done1) }()
	j, _, _ := s.Enqueue(context.Background(), job(tid, "w"))
	<-started
	stop1()
	<-done1
	if g, _ := s.Get(context.Background(), j.ID); g.Status != Queued || g.Attempts != 0 {
		t.Fatalf("not released on shutdown: %+v", g)
	}
	ctx2, stop2 := context.WithCancel(context.Background())
	defer stop2()
	r2 := NewRunner(s, fast)
	r2.Register(KindWorkspaceProvision, func(context.Context, *Job, func(string)) (any, error) { return nil, nil })
	go r2.Run(ctx2)
	if got := waitDone(t, s, j.ID); got.Status != Succeeded {
		t.Fatalf("got %+v", got)
	}
}

// A handler that loses its lease is canceled and its outcome is dropped.
func TestRunnerLostLeaseCancels(t *testing.T) {
	s, d, tid := testJobs(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	canceled := make(chan struct{})
	r := NewRunner(s, fast)
	r.Register(KindWorkspaceProvision, func(ctx context.Context, j *Job, _ func(string)) (any, error) {
		_, _ = d.Pool.Exec(context.Background(), `UPDATE jobs SET lease_owner='other' WHERE id=$1`, j.ID)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	go r.Run(ctx)
	j, _, _ := s.Enqueue(ctx, job(tid, "w"))
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("handler not canceled after losing the lease")
	}
	time.Sleep(100 * time.Millisecond)
	if g, _ := s.Get(context.Background(), j.ID); g.Status != Running {
		t.Fatalf("the losing worker changed the job: %+v", g)
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	r := &Runner{set: Settings{BackoffBase: time.Second, BackoffMax: 5 * time.Second}}
	for attempt, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 5 * time.Second, 9: 5 * time.Second} {
		if got := r.backoff(attempt); got != want {
			t.Errorf("backoff(%d)=%v want %v", attempt, got, want)
		}
	}
}
