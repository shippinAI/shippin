package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// StaffRole is one staff role (data; synced to a Nomen project role with the same key).
type StaffRole struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	Preset      bool      `json:"preset"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StaffSettings are the team settings (one row, edited in the Team page, every change audited).
type StaffSettings struct {
	Mode                string `json:"mode"`                  // legacy | shadow | enforce
	LegacyFallback      bool   `json:"legacy_fallback"`       // enforce: IAM owners keep Owner rights (logged)
	TwoPerson           bool   `json:"two_person"`            // high-risk team changes need a second Owner (2+ Owners)
	IdleDays            int    `json:"idle_days"`             // flag staff idle longer than this
	BreakGlassApproval  bool   `json:"breakglass_approval"`   // break-glass waits for an approver
	ApprovalMinutes     int    `json:"approval_minutes"`      // pending break-glass lapses after this
	SoleOwnerActivation bool   `json:"sole_owner_activation"` // no other approver: an Owner may self-activate (flagged)
}

// ErrNotFound is returned when a staff row doesn't exist.
var ErrNotFound = errors.New("not found")

// SeedStaff inserts presets and default settings once, on a database without team settings.
func (d *DB) SeedStaff(ctx context.Context) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	b, _ := json.Marshal(DefaultStaffSettings)
	tag, err := tx.Exec(ctx, `INSERT INTO staff_settings(id, data, updated_by) VALUES(1, $1, 'seed') ON CONFLICT DO NOTHING`, b)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	for _, r := range StaffPresets {
		if _, err := tx.Exec(ctx, `INSERT INTO staff_roles(key, name, description, permissions, preset)
			VALUES($1, $2, $3, $4, true) ON CONFLICT DO NOTHING`, r.Key, r.Name, r.Description, r.Permissions); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// StaffRoles lists every role by name.
func (d *DB) StaffRoles(ctx context.Context) ([]StaffRole, error) {
	rows, err := d.Pool.Query(ctx, `SELECT key, name, description, permissions, preset, updated_at FROM staff_roles ORDER BY preset DESC, created_at, key`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (StaffRole, error) {
		var s StaffRole
		return s, r.Scan(&s.Key, &s.Name, &s.Description, &s.Permissions, &s.Preset, &s.UpdatedAt)
	})
}

// SaveStaffRole inserts (create) or updates a role and its audit entries in one transaction.
func (d *DB) SaveStaffRole(ctx context.Context, r StaffRole, create bool, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		q := `UPDATE staff_roles SET name=$2, description=$3, permissions=$4, updated_at=now() WHERE key=$1`
		if create {
			q = `INSERT INTO staff_roles(key, name, description, permissions) VALUES($1, $2, $3, $4) ON CONFLICT DO NOTHING`
		}
		tag, err := tx.Exec(ctx, q, r.Key, r.Name, r.Description, r.Permissions)
		return tag.RowsAffected(), err
	})
}

// DeleteStaffRole removes a role, audited in the same transaction.
func (d *DB) DeleteStaffRole(ctx context.Context, key string, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `DELETE FROM staff_roles WHERE key=$1`, key)
		return tag.RowsAffected(), err
	})
}

// RestoreStaffRole puts a role back exactly (compensation when the IdP write failed).
func (d *DB) RestoreStaffRole(ctx context.Context, r StaffRole, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `INSERT INTO staff_roles(key, name, description, permissions, preset) VALUES($1, $2, $3, $4, $5)
			ON CONFLICT (key) DO UPDATE SET name=$2, description=$3, permissions=$4, preset=$5, updated_at=now()`,
			r.Key, r.Name, r.Description, r.Permissions, r.Preset)
		return tag.RowsAffected(), err
	})
}

// inTx runs write, requires exactly one affected row, and appends es; audit and change commit together.
func (d *DB) inTx(ctx context.Context, es []AuditEntry, write func(pgx.Tx) (int64, error)) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n, err := write(tx)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err := addAuditTx(ctx, tx, es...); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
