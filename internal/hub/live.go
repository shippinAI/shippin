package hub

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Change is one hub_events notification (migration 029), fanned out to the org's boards.
type Change struct {
	OrgID   string `json:"org_id"`
	TaskID  string `json:"task_id"`
	EventID int64  `json:"event_id"`
	Kind    string `json:"kind"`
}

// Live listens on Postgres (LISTEN hub_events) and fans changes out per org. Notifications
// come from the database, so every console replica sees every replica's writes.
type Live struct {
	mu   sync.Mutex
	subs map[chan Change]string
}

// NewLive returns an idle broker; Run starts the listener.
func NewLive() *Live { return &Live{subs: map[chan Change]string{}} }

// Run listens until ctx ends, reconnecting after errors.
func (l *Live) Run(ctx context.Context, pool *pgxpool.Pool) {
	for ctx.Err() == nil {
		if err := l.listen(ctx, pool); err != nil && ctx.Err() == nil {
			log.Printf("hub live: %v (retrying)", err)
			time.Sleep(2 * time.Second)
		}
	}
}

func (l *Live) listen(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN hub_events"); err != nil {
		return err
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var c Change
		if json.Unmarshal([]byte(n.Payload), &c) == nil {
			l.publish(c)
		}
	}
}

func (l *Live) publish(c Change) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for ch, org := range l.subs {
		if org != c.OrgID {
			continue
		}
		select {
		case ch <- c:
		default: // a slow board misses a nudge; its next change (or poll) catches it up
		}
	}
}

// Subscribe returns a channel of org's changes and its cancel func.
func (l *Live) Subscribe(org string) (<-chan Change, func()) {
	ch := make(chan Change, 16)
	l.mu.Lock()
	l.subs[ch] = org
	l.mu.Unlock()
	return ch, func() {
		l.mu.Lock()
		delete(l.subs, ch)
		l.mu.Unlock()
	}
}
