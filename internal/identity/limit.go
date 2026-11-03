package identity

import (
	"sync"
	"time"
)

// Limiter counts events per key inside a window. It is per process.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, hits: map[string][]time.Time{}}
}

// Allow reports whether key may proceed at now.
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
