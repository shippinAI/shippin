package api

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"shippin.cloud/console/internal/db"
)

// fakeStore is an in-memory staffStore. failAudit makes every audit write fail.
type fakeStore struct {
	mu        sync.Mutex
	roles     []db.StaffRole
	settings  db.StaffSettings
	audit     []db.AuditEntry
	requests  []db.StaffRequest
	failAudit bool
}

func newFakeStore(mode string) *fakeStore {
	s := db.DefaultStaffSettings
	s.Mode, s.LegacyFallback = mode, false
	return &fakeStore{roles: slices.Clone(db.StaffPresets), settings: s}
}

var errFake = errors.New("fake failure")

func (f *fakeStore) addAudit(es []db.AuditEntry) error {
	if f.failAudit {
		return errFake
	}
	f.audit = append(f.audit, es...)
	return nil
}
func (f *fakeStore) StaffRoles(context.Context) ([]db.StaffRole, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.roles), nil
}
func (f *fakeStore) idx(key string) int {
	return slices.IndexFunc(f.roles, func(r db.StaffRole) bool { return r.Key == key })
}
func (f *fakeStore) SaveStaffRole(_ context.Context, r db.StaffRole, create bool, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.idx(r.Key)
	if (create && i >= 0) || (!create && i < 0) {
		return db.ErrNotFound
	}
	if err := f.addAudit(es); err != nil {
		return err
	}
	if create {
		f.roles = append(f.roles, r)
	} else {
		r.Preset = f.roles[i].Preset
		f.roles[i] = r
	}
	return nil
}
func (f *fakeStore) DeleteStaffRole(_ context.Context, key string, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.idx(key)
	if i < 0 {
		return db.ErrNotFound
	}
	if err := f.addAudit(es); err != nil {
		return err
	}
	f.roles = slices.Delete(f.roles, i, i+1)
	return nil
}
func (f *fakeStore) RestoreStaffRole(_ context.Context, r db.StaffRole, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := f.idx(r.Key); i >= 0 {
		f.roles[i] = r
	} else {
		f.roles = append(f.roles, r)
	}
	return f.addAudit(es)
}
func (f *fakeStore) StaffSettings(context.Context) (db.StaffSettings, error) { return f.settings, nil }
func (f *fakeStore) SaveStaffSettings(_ context.Context, s db.StaffSettings, _ string, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.addAudit(es); err != nil {
		return err
	}
	f.settings = s
	return nil
}
func (f *fakeStore) TouchStaff(context.Context, string) error { return nil }
func (f *fakeStore) StaffSeen(context.Context) (map[string]time.Time, error) {
	return map[string]time.Time{}, nil
}
func (f *fakeStore) CreateStaffRequest(_ context.Context, q db.StaffRequest, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.addAudit(es); err != nil {
		return err
	}
	q.Status = "pending"
	f.requests = append(f.requests, q)
	return nil
}
func (f *fakeStore) StaffRequests(context.Context) ([]db.StaffRequest, error) { return f.requests, nil }
func (f *fakeStore) StaffRequestByID(_ context.Context, id string) (*db.StaffRequest, error) {
	for i := range f.requests {
		if f.requests[i].ID == id {
			q := f.requests[i]
			return &q, nil
		}
	}
	return nil, db.ErrNotFound
}
func (f *fakeStore) DecideStaffRequest(_ context.Context, id, status, _ string, es ...db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.requests {
		if f.requests[i].ID == id && f.requests[i].Status == "pending" {
			if err := f.addAudit(es); err != nil {
				return err
			}
			f.requests[i].Status = status
			return nil
		}
	}
	return db.ErrNotFound
}
func (f *fakeStore) AddAudit(_ context.Context, e db.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addAudit([]db.AuditEntry{e})
}
func (f *fakeStore) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, e := range f.audit {
		out = append(out, e.Action)
	}
	return out
}
