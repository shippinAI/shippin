package api

import (
	"context"
	"slices"
	"sync"

	"shippin.cloud/console/internal/db"
)

// fakeGrants is an in-memory permGrantStore; audit entries go to the staff fake.
type fakeGrants struct {
	mu    sync.Mutex
	gs    []db.PermGrant
	store *fakeStore
}

func (f *fakeGrants) PermGrants(context.Context) ([]db.PermGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.gs), nil
}
func (f *fakeGrants) PermGrantsOf(_ context.Context, s string) ([]db.PermGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []db.PermGrant{}
	for _, g := range f.gs {
		if g.Subject == s {
			out = append(out, g)
		}
	}
	return out, nil
}
func (f *fakeGrants) GrantPerm(_ context.Context, g db.PermGrant, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if slices.ContainsFunc(f.gs, func(x db.PermGrant) bool { return x.Subject == g.Subject && x.Permission == g.Permission }) {
		return db.ErrNotFound
	}
	f.gs = append(f.gs, g)
	return f.store.AddAudit(context.Background(), es[0])
}
func (f *fakeGrants) RevokePerm(_ context.Context, s, p string, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.gs, func(x db.PermGrant) bool { return x.Subject == s && x.Permission == p })
	if i < 0 {
		return db.ErrNotFound
	}
	f.gs = slices.Delete(f.gs, i, i+1)
	return f.store.AddAudit(context.Background(), es[0])
}
