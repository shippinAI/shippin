package tenantdb

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Backup is a one-click backup request. Status pending_backend means no dump
// has been taken; the Postgres backup service is not wired yet.
type Backup struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Actor     string    `json:"actor"`
	Note      string    `json:"note,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// BackupStore is the one-click backup surface (list, create, restore).
// TODO: replace the placeholder with hostd running pg_dump / pg_restore as the
// tenant role, storing the dump on that tenant's dataset only.
type BackupStore interface {
	List(ctx context.Context, tenantID string) ([]Backup, error)
	Create(ctx context.Context, tenantID, actor, note string) (Backup, error)
	Restore(ctx context.Context, tenantID, backupID string) error
}

// ErrBackupPending is returned by Restore until a real backend exists.
var ErrBackupPending = errors.New("backups are not connected to Postgres yet")

// ErrBackupNotFound is returned for a missing id and for another tenant's id.
var ErrBackupNotFound = errors.New("backup not found")

// TODOStore records backup intent in memory and never dumps or restores data.
type TODOStore struct {
	mu    sync.Mutex
	items []Backup
	Now   func() time.Time
}

func (s *TODOStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// List returns this tenant's recorded requests only.
func (s *TODOStore) List(_ context.Context, tenantID string) ([]Backup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Backup{}
	for _, b := range s.items {
		if b.TenantID == tenantID {
			out = append(out, b)
		}
	}
	return out, nil
}

// Create records a request. It does not copy any table.
func (s *TODOStore) Create(_ context.Context, tenantID, actor, note string) (Backup, error) {
	if _, err := TargetOf(tenantID); err != nil {
		return Backup{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Backup{}, err
	}
	b := Backup{ID: id.String(), TenantID: tenantID, Actor: actor, Note: note, Status: "pending_backend", CreatedAt: s.now()}
	s.mu.Lock()
	s.items = append(s.items, b)
	s.mu.Unlock()
	return b, nil
}

// Restore refuses to change data. A matching row for another tenant is hidden.
func (s *TODOStore) Restore(_ context.Context, tenantID, backupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.items {
		if b.ID == backupID && b.TenantID == tenantID {
			return ErrBackupPending
		}
	}
	return ErrBackupNotFound
}
