package db

import (
	"context"
	"errors"
)

// ErrUnknownTransition guards against typos reaching SQL.
var ErrUnknownTransition = errors.New("unknown grant transition")

// transitions: the only status changes a grant may make, each from fixed source states.
// approve and deny record the deciding customer; the others are recorded in the audit log.
var transitions = map[string]string{
	"approve": `SET status='approved', decided_by=$2, decided_name=$3, decided_at=now(),
		expires_at=now()+duration_s*interval '1 second' WHERE id::text=$1 AND status='pending'`,
	"deny":   `SET status='denied', decided_by=$2, decided_name=$3, decided_at=now() WHERE id::text=$1 AND status='pending'`,
	"cancel": `SET status='cancelled', ended_at=now() WHERE id::text=$1 AND status='pending'`,
	"revoke": `SET status='revoked', ended_at=now() WHERE id::text=$1 AND status='approved' AND expires_at>now()`,
	"end":    `SET status='ended', ended_at=now() WHERE id::text=$1 AND status='approved' AND expires_at>now()`,
}

// CreateGrant inserts a grant and its audit entries in one transaction (no grant without audit).
// A break-glass grant is inserted already approved, expiring after its duration.
func (d *DB) CreateGrant(ctx context.Context, g Grant, es ...AuditEntry) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO access_grants(id, tenant_id, workspace_id, kind, requester, requester_name,
		reason, category, duration_s, status, decided_by, decided_name, decided_at, expires_at, notice_at)
		VALUES($1, $2, nullif($3,'')::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12,
		CASE WHEN $10='approved' THEN now() END,
		CASE WHEN $10='approved' THEN now()+$9::int*interval '1 second' END, $13)`,
		g.ID, g.TenantID, g.WorkspaceID, g.Kind, g.Requester, g.RequesterName, g.Reason, g.Category,
		g.DurationS, g.Status, g.DecidedBy, g.DecidedName, g.NoticeAt); err != nil {
		return err
	}
	if err := addAuditTx(ctx, tx, es...); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// TransitionGrant applies op atomically with its audit entries; false when the grant wasn't in
// a state op may leave (already decided, expired, ...), in which case nothing is written.
func (d *DB) TransitionGrant(ctx context.Context, id, op, by, byName string, es ...AuditEntry) (bool, error) {
	set, ok := transitions[op]
	if !ok {
		return false, ErrUnknownTransition
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	args := []any{id}
	if op == "approve" || op == "deny" {
		args = append(args, by, byName)
	}
	tag, err := tx.Exec(ctx, `UPDATE access_grants `+set, args...)
	if err != nil || tag.RowsAffected() != 1 {
		return false, err
	}
	if err := addAuditTx(ctx, tx, es...); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// ExpireGrants closes approved grants past expiry and audits each via entry, atomically.
func (d *DB) ExpireGrants(ctx context.Context, entry func(Grant) AuditEntry) ([]Grant, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH x AS (UPDATE access_grants SET status='expired', ended_at=expires_at
		WHERE status='approved' AND expires_at<=now() RETURNING *) SELECT `+
		grantCols+` FROM x g JOIN tenants t ON t.id=g.tenant_id LEFT JOIN workspaces w ON w.id=g.workspace_id`)
	if err != nil {
		return nil, err
	}
	gs, err := scanGrants(rows)
	if err != nil {
		return nil, err
	}
	for _, g := range gs {
		if err := addAuditTx(ctx, tx, entry(g)); err != nil {
			return nil, err
		}
	}
	return gs, tx.Commit(ctx)
}
