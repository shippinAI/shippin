package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/tenantdb"
)

// SQLAudit is one console SQL attempt. Status starts as "started" and becomes
// ok, error, or rejected. Statement text is capped; row values are not stored.
type SQLAudit struct {
	ID        string    `json:"id" db:"id"`
	TenantID  string    `json:"tenant_id" db:"tenant_id"`
	Actor     string    `json:"actor" db:"actor"`
	ActorName string    `json:"actor_name" db:"actor_name"`
	Action    string    `json:"action" db:"action"`
	Statement string    `json:"statement" db:"statement"`
	Kind      string    `json:"kind" db:"kind"`
	ReadOnly  bool      `json:"read_only" db:"read_only"`
	Confirmed bool      `json:"confirmed" db:"confirmed"`
	RowCount  *int      `json:"row_count" db:"row_count"`
	Error     string    `json:"error" db:"error"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// SQLSnippet is a saved statement for one person in one tenant.
type SQLSnippet struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Owner     string    `json:"owner"`
	Name      string    `json:"name"`
	SQL       string    `json:"sql"`
	UpdatedAt time.Time `json:"updated_at"`
}

func clipSQL(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 8000 {
		return s[:8000]
	}
	return s
}

// BeginSQLAudit records an attempt before it runs. A failure here must stop the query.
func (d *DB) BeginSQLAudit(ctx context.Context, e SQLAudit) (string, error) {
	if e.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		e.ID = id.String()
	}
	if e.Status == "" {
		e.Status = "started"
	}
	_, err := d.Pool.Exec(ctx, `INSERT INTO tenant_sql_audit
		(id, tenant_id, actor, actor_name, action, statement, kind, read_only, confirmed, error, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.ID, e.TenantID, e.Actor, e.ActorName, e.Action, clipSQL(e.Statement), e.Kind, e.ReadOnly, e.Confirmed, e.Error, e.Status)
	return e.ID, err
}

// FinishSQLAudit writes the outcome. The tenant id is part of the match so one
// tenant cannot close another's row.
func (d *DB) FinishSQLAudit(ctx context.Context, tenantID, id, status, errText string, rows int) error {
	var n any
	if status == "ok" {
		n = rows
	}
	tag, err := d.Pool.Exec(ctx, `UPDATE tenant_sql_audit SET status=$3, error=$4, row_count=$5
		WHERE id=$1 AND tenant_id=$2`, id, tenantID, status, errText, n)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("audit row not found")
	}
	return nil
}

// ListSQLAudit returns the newest attempts for one tenant.
func (d *DB) ListSQLAudit(ctx context.Context, tenantID string, limit int) ([]SQLAudit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := d.Pool.Query(ctx, `SELECT id::text, tenant_id::text, actor, actor_name, action, statement, kind,
		read_only, confirmed, row_count, error, status, created_at
		FROM tenant_sql_audit WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SQLAudit{}
	for rows.Next() {
		var e SQLAudit
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Actor, &e.ActorName, &e.Action, &e.Statement, &e.Kind,
			&e.ReadOnly, &e.Confirmed, &e.RowCount, &e.Error, &e.Status, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListSnippets returns snippets saved by owner inside the tenant.
func (d *DB) ListSnippets(ctx context.Context, tenantID, owner string) ([]SQLSnippet, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id::text, tenant_id::text, owner_sub, name, sql, updated_at
		FROM tenant_sql_snippets WHERE tenant_id=$1 AND owner_sub=$2 ORDER BY updated_at DESC`, tenantID, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SQLSnippet{}
	for rows.Next() {
		var s SQLSnippet
		if err := rows.Scan(&s.ID, &s.TenantID, &s.Owner, &s.Name, &s.SQL, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveSnippet inserts or updates a snippet owned by owner.
func (d *DB) SaveSnippet(ctx context.Context, s SQLSnippet) (SQLSnippet, error) {
	if s.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return SQLSnippet{}, err
		}
		s.ID = id.String()
	}
	err := d.Pool.QueryRow(ctx, `INSERT INTO tenant_sql_snippets (id, tenant_id, owner_sub, name, sql)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id, owner_sub, name) DO UPDATE SET sql=EXCLUDED.sql, updated_at=now()
		RETURNING id::text, updated_at`, s.ID, s.TenantID, s.Owner, s.Name, s.SQL).Scan(&s.ID, &s.UpdatedAt)
	return s, err
}

// DeleteSnippet removes one snippet. Another person's id is a no-op success
// only when nothing matched; the caller treats 0 rows as not found.
func (d *DB) DeleteSnippet(ctx context.Context, tenantID, owner, id string) error {
	tag, err := d.Pool.Exec(ctx, `DELETE FROM tenant_sql_snippets WHERE id=$1 AND tenant_id=$2 AND owner_sub=$3`, id, tenantID, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("snippet not found")
	}
	return nil
}

// TenantBackups is the console record of backup requests.
// TODO: Create does not dump Postgres. Restore does not change data.
// Wire hostd to pg_dump / pg_restore as the tenant role, then replace Restore.
type TenantBackups struct{ Pool *pgxpool.Pool }

func (s TenantBackups) List(ctx context.Context, tenantID string) ([]tenantdb.Backup, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, tenant_id::text, actor, note, status, created_at
		FROM tenant_db_backups WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tenantdb.Backup{}
	for rows.Next() {
		var b tenantdb.Backup
		if err := rows.Scan(&b.ID, &b.TenantID, &b.Actor, &b.Note, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s TenantBackups) Create(ctx context.Context, tenantID, actor, note string) (tenantdb.Backup, error) {
	if _, err := tenantdb.TargetOf(tenantID); err != nil {
		return tenantdb.Backup{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return tenantdb.Backup{}, err
	}
	b := tenantdb.Backup{ID: id.String(), TenantID: tenantID, Actor: actor, Note: note, Status: "pending_backend", CreatedAt: time.Now().UTC()}
	_, err = s.Pool.Exec(ctx, `INSERT INTO tenant_db_backups (id, tenant_id, actor, note, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, b.ID, b.TenantID, b.Actor, b.Note, b.Status, b.CreatedAt)
	return b, err
}

func (s TenantBackups) Restore(ctx context.Context, tenantID, backupID string) error {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_db_backups WHERE id=$1 AND tenant_id=$2`, backupID, tenantID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenantdb.ErrBackupNotFound
	}
	if err != nil {
		return err
	}
	// TODO: hostd pg_restore as tn_* onto tdb_* only.
	return tenantdb.ErrBackupPending
}
