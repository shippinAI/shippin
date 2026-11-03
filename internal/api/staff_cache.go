package api

import (
	"context"
	"sync"
	"time"

	"shippin.cloud/console/internal/db"
)

type staffEntry struct {
	v  *staffView
	at time.Time
}

// staffCache caches per-user views and the roles/settings tables for a few seconds.
type staffCache struct {
	mu       sync.Mutex
	users    map[string]staffEntry
	roles    []db.StaffRole
	rolesAt  time.Time
	settings *db.StaffSettings
	setAt    time.Time
}

// flush drops everything after a team change so it applies on the next request.
func (s *staffCache) flush() {
	s.mu.Lock()
	s.users, s.roles, s.settings = nil, nil, nil
	s.mu.Unlock()
}

func (a *API) staffTTL() time.Duration {
	if a.cfg.Staff.CacheSeconds > 0 {
		return time.Duration(a.cfg.Staff.CacheSeconds) * time.Second
	}
	return 30 * time.Second
}

// staffRoles is the roles table (cached briefly).
func (a *API) staffRoles(ctx context.Context) ([]db.StaffRole, error) {
	s := &a.staffCache
	s.mu.Lock()
	if s.roles != nil && time.Since(s.rolesAt) < 5*time.Second {
		r := s.roles
		s.mu.Unlock()
		return r, nil
	}
	s.mu.Unlock()
	rs, err := a.store().StaffRoles(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.roles, s.rolesAt = rs, time.Now()
	s.mu.Unlock()
	return rs, nil
}
