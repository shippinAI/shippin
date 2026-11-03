package jobs

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Handler does one job. step records progress; ctx is canceled when the lease is lost or on shutdown.
// Handlers must be idempotent: a job whose worker died is run again from the start.
type Handler func(ctx context.Context, j *Job, step func(string)) (result any, err error)

// Settings tune the runner (from config "jobs").
type Settings struct {
	Workers                 int
	Lease, Heartbeat, Poll  time.Duration
	BackoffBase, BackoffMax time.Duration
}

// Runner claims jobs and runs their handlers.
type Runner struct {
	store    *Store
	set      Settings
	owner    string
	handlers map[string]Handler
	wake     chan struct{}
}

// NewRunner builds a runner; the lease owner is host/pid/random so replicas never collide.
func NewRunner(s *Store, set Settings) *Runner {
	host, _ := os.Hostname()
	id, _ := uuid.NewRandom()
	return &Runner{store: s, set: set, handlers: map[string]Handler{}, wake: make(chan struct{}, 1),
		owner: fmt.Sprintf("%s/%d/%s", host, os.Getpid(), id.String()[:8])}
}

// Register adds the handler for a kind (before Run).
func (r *Runner) Register(kind string, h Handler) { r.handlers[kind] = h }

// Wake makes an idle worker poll now (call after Enqueue).
func (r *Runner) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run starts the workers and blocks until ctx is done and every worker has stopped.
func (r *Runner) Run(ctx context.Context) {
	kinds := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	var wg sync.WaitGroup
	for range max(r.set.Workers, 1) {
		wg.Add(1)
		go func() { defer wg.Done(); r.work(ctx, kinds) }()
	}
	wg.Wait()
}

func (r *Runner) work(ctx context.Context, kinds []string) {
	for ctx.Err() == nil {
		j, err := r.claim(ctx, kinds)
		if err != nil && ctx.Err() == nil {
			log.Printf("jobs: claim: %v", err)
		}
		if j != nil {
			r.runOne(ctx, j)
			continue
		}
		select {
		case <-ctx.Done():
		case <-r.wake:
		case <-time.After(r.set.Poll):
		}
	}
}

// backoff doubles from BackoffBase per attempt, capped at BackoffMax.
func (r *Runner) backoff(attempt int) time.Duration {
	d := r.set.BackoffBase
	for i := 1; i < attempt && d < r.set.BackoffMax; i++ {
		d *= 2
	}
	return min(d, r.set.BackoffMax)
}

// Each claim gets its own fencing token, including recovery by this same runner.
func (r *Runner) claim(ctx context.Context, kinds []string) (*Job, error) {
	return r.store.Claim(ctx, r.owner+"/"+uuid.NewString(), kinds, r.set.Lease)
}
