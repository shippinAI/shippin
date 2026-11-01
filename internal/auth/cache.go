package auth

import (
	"crypto/sha256"
	"sync"
	"time"
)

// idCache keeps IdP lookups for 60 s, keyed by a hash of the token (forward-auth runs per request).
type idCache struct {
	mu sync.Mutex
	m  map[[32]byte]cached
}

type cached struct {
	id  *Identity
	exp time.Time
}

func newIDCache() *idCache { return &idCache{m: map[[32]byte]cached{}} }

func (c *idCache) get(tok string) *Identity {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[sha256.Sum256([]byte(tok))]
	if !ok || time.Now().After(e.exp) {
		return nil
	}
	return e.id
}

func (c *idCache) put(tok string, id *Identity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 5000 {
		c.m = map[[32]byte]cached{}
	}
	c.m[sha256.Sum256([]byte(tok))] = cached{id, time.Now().Add(60 * time.Second)}
}

func (c *idCache) drop(tok string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, sha256.Sum256([]byte(tok)))
}
