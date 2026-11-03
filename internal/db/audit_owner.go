package db

import (
	"context"
	_ "embed"
)

// AuditOwnerSQL moves audit_log and access_grants to the NOLOGIN shippin_audit_owner role
// (see the file header). deploy/audit-owner.sh runs the same file on the real database.
//
//go:embed audit_owner.sql
var AuditOwnerSQL string

// AuditChainSQL adds the audit hash chain (see the file header); it runs after AuditOwnerSQL.
//
//go:embed audit_chain.sql
var AuditChainSQL string

// HardenAudit applies AuditOwnerSQL and then AuditChainSQL for appRole. The connection must be a superuser's.
func HardenAudit(ctx context.Context, d *DB, appRole string) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('shippin.app_role', $1, true)`, appRole); err != nil {
		return err
	}
	for _, q := range []string{AuditOwnerSQL, AuditChainSQL} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
