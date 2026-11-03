package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// PermGrant is one permission granted to one principal directly (not through a role).
type PermGrant struct {
	Subject    string    `json:"subject"`
	Permission string    `json:"permission"`
	Reason     string    `json:"reason"`
	GrantedBy  string    `json:"granted_by"`
	GrantedAt  time.Time `json:"granted_at"`
}

// PermGrants lists every direct grant (Team > Direct grants).
func (d *DB) PermGrants(ctx context.Context) ([]PermGrant, error) {
	return d.permGrants(ctx, `ORDER BY granted_at`)
}

// PermGrantsOf lists one principal's direct grants (resolved with their roles).
func (d *DB) PermGrantsOf(ctx context.Context, subject string) ([]PermGrant, error) {
	return d.permGrants(ctx, `WHERE subject=$1 ORDER BY permission`, subject)
}

func (d *DB) permGrants(ctx context.Context, where string, args ...any) ([]PermGrant, error) {
	rows, err := d.Pool.Query(ctx, `SELECT subject, permission, reason, granted_by, granted_at FROM staff_perm_grants `+where, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (PermGrant, error) {
		var g PermGrant
		return g, r.Scan(&g.Subject, &g.Permission, &g.Reason, &g.GrantedBy, &g.GrantedAt)
	})
}

// GrantPerm adds a direct grant, audited in the same transaction; ErrNotFound = already granted.
func (d *DB) GrantPerm(ctx context.Context, g PermGrant, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `INSERT INTO staff_perm_grants(subject, permission, reason, granted_by) VALUES($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, g.Subject, g.Permission, g.Reason, g.GrantedBy)
		return tag.RowsAffected(), err
	})
}

// RevokePerm removes a direct grant, audited in the same transaction; ErrNotFound = no such grant.
func (d *DB) RevokePerm(ctx context.Context, subject, perm string, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `DELETE FROM staff_perm_grants WHERE subject=$1 AND permission=$2`, subject, perm)
		return tag.RowsAffected(), err
	})
}
