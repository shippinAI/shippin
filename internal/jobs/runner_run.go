package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// runOne runs a claimed job under a heartbeat and records the outcome.
func (r *Runner) runOne(ctx context.Context, j *Job) {
	bg := context.WithoutCancel(ctx)
	// A committed lifecycle receipt takes precedence over the retry budget. No provider
	// handler is invoked to replay it, so recovery cannot dispatch another action.
	if j.Kind == KindWorkspaceAction {
		var in struct {
			Completion *struct {
				State   string `json:"state"`
				Message string `json:"message"`
			} `json:"completion"`
		}
		if json.Unmarshal(j.Payload, &in) == nil && in.Completion != nil {
			if in.Completion.State == "failed" {
				_, _ = r.store.Fail(bg, j.ID, j.LeaseOwner, in.Completion.Message, false, 0)
			} else {
				_ = r.store.Succeed(bg, j.ID, j.LeaseOwner, map[string]string{"state": in.Completion.State})
			}
			return
		}
	}
	if j.Attempts > j.MaxAttempts { // its workers kept dying: stop retrying
		_, _ = r.store.Fail(bg, j.ID, j.LeaseOwner, fmt.Sprintf("gave up after %d attempts", j.MaxAttempts), false, 0)
		return
	}
	jctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lost := make(chan struct{})
	go r.heartbeat(jctx, j.ID, j.LeaseOwner, cancel, lost)
	step := func(s string) {
		if err := r.store.Step(bg, j.ID, j.LeaseOwner, s); err != nil {
			log.Printf("jobs: %s step: %v", j.ID, err)
		}
	}
	res, err := r.safeRun(jctx, j, step)
	cancel()
	select {
	case <-lost: // another worker owns it now; its outcome wins
		log.Printf("jobs: %s %s lost its lease", j.Kind, j.ID)
		return
	default:
	}
	switch {
	case err == nil:
		if e := r.store.Succeed(bg, j.ID, j.LeaseOwner, res); e != nil {
			log.Printf("jobs: %s succeed: %v", j.ID, e)
		}
	case ctx.Err() != nil: // shutdown: hand it back for the next start
		_ = r.store.Release(bg, j.ID, j.LeaseOwner)
	default:
		final, e := r.store.Fail(bg, j.ID, j.LeaseOwner, err.Error(), !IsPermanent(err), r.backoff(j.Attempts))
		if e != nil {
			log.Printf("jobs: %s fail: %v", j.ID, e)
		}
		log.Printf("jobs: %s %s attempt %d failed (final=%v): %v", j.Kind, j.ID, j.Attempts, final, err)
	}
}

// safeRun turns a handler panic into a retryable error.
func (r *Runner) safeRun(ctx context.Context, j *Job, step func(string)) (res any, err error) {
	h, ok := r.handlers[j.Kind]
	if !ok {
		return nil, Permanent(fmt.Errorf("no handler for %s", j.Kind))
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return h(ctx, j, step)
}

// heartbeat renews the lease until ctx ends; a lost lease cancels the handler.
func (r *Runner) heartbeat(ctx context.Context, id, owner string, cancel context.CancelFunc, lost chan struct{}) {
	t := time.NewTicker(r.set.Heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := r.store.Heartbeat(ctx, id, owner, r.set.Lease); err == ErrLostLease {
				close(lost)
				cancel()
				return
			}
		}
	}
}
