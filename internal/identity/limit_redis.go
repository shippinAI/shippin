package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

// SharedLimits stores short-lived authentication counters in Dragonfly or Redis.
// PostgreSQL remains the source of truth for accounts, sessions and durable jobs.
type SharedLimits struct {
	client *redis.Client
	prefix string
}

func NewSharedLimits(ctx context.Context, address, prefix string) (*SharedLimits, error) {
	opts, err := redis.ParseURL(address)
	if err != nil {
		return nil, err
	}
	opts.DialTimeout = 2 * time.Second
	opts.ReadTimeout = 2 * time.Second
	opts.WriteTimeout = 2 * time.Second
	opts.MaxRetries = -1
	opts.Protocol = 2
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}
	return &SharedLimits{client: client, prefix: prefix}, nil
}
func (s *SharedLimits) Close() error                   { return s.client.Close() }
func (s *SharedLimits) Ping(ctx context.Context) error { return s.client.Ping(ctx).Err() }
func (s *Service) UseSharedLimits(shared *SharedLimits) {
	s.limit = &sharedLimiter{shared, "public", 5, 10 * time.Minute}
	s.passkeyLimit = &sharedLimiter{shared, "passkey", 30, time.Minute}
	s.passwordLimit = &sharedLimiter{shared, "password", 8, 10 * time.Minute}
}

type sharedLimiter struct {
	shared *SharedLimits
	scope  string
	limit  int
	window time.Duration
}

var incrementWindow = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n
`)

// Each fixed window starts on its first attempt. A configured store failure denies
// attempts; silently switching to per-process limits would bypass a shared limit.
func (l *sharedLimiter) Allow(key string, _ time.Time) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	digest := sha256.Sum256([]byte(key))
	name := l.shared.prefix + "auth:" + l.scope + ":" + hex.EncodeToString(digest[:])
	count, err := incrementWindow.Run(ctx, l.shared.client, []string{name}, l.window.Milliseconds()).Int64()
	return err == nil && count <= int64(l.limit)
}
