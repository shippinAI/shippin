package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// AuditEntry is one append-only audit row (the table rejects UPDATE, DELETE and TRUNCATE).
type AuditEntry struct {
	ID            int64     `json:"id"`
	At            time.Time `json:"at"`
	TenantID      string    `json:"tenant_id,omitempty"`
	TenantName    string    `json:"tenant_name,omitempty"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	WorkspaceName string    `json:"workspace_name,omitempty"`
	GrantID       string    `json:"grant_id,omitempty"`
	Actor         string    `json:"actor"`
	ActorName     string    `json:"actor_name"`
	Action        string    `json:"action"`
	Detail        string    `json:"detail,omitempty"`
	IP            string    `json:"ip,omitempty"`
	BreakGlass    bool      `json:"breakglass"`
}

const auditInsert = `INSERT INTO audit_log(tenant_id, workspace_id, grant_id, actor, actor_name, action, detail, ip, breakglass)
	VALUES(nullif($1,'')::uuid, nullif($2,'')::uuid, nullif($3,'')::uuid, $4, $5, $6, $7, $8, $9)`

func auditArgs(e AuditEntry) []any {
	return []any{e.TenantID, e.WorkspaceID, e.GrantID, e.Actor, e.ActorName, e.Action, e.Detail, e.IP, e.BreakGlass}
}

// AddAudit appends one entry.
func (d *DB) AddAudit(ctx context.Context, e AuditEntry) error {
	_, err := d.Pool.Exec(ctx, auditInsert, auditArgs(e)...)
	return err
}

func addAuditTx(ctx context.Context, tx pgx.Tx, es ...AuditEntry) error {
	for _, e := range es {
		if _, err := tx.Exec(ctx, auditInsert, auditArgs(e)...); err != nil {
			return err
		}
	}
	return nil
}

const auditSelect = `SELECT a.id, a.at, coalesce(a.tenant_id::text,''), coalesce(t.name,''), coalesce(a.workspace_id::text,''),
	coalesce(w.name,''), coalesce(a.grant_id::text,''), a.actor, a.actor_name, a.action, a.detail, a.ip, a.breakglass
	FROM audit_log a LEFT JOIN tenants t ON t.id=a.tenant_id LEFT JOIN workspaces w ON w.id=a.workspace_id
	LEFT JOIN access_grants g ON g.id=a.grant_id `

func (d *DB) audit(ctx context.Context, where string, args ...any) ([]AuditEntry, error) {
	rows, err := d.Pool.Query(ctx, auditSelect+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.TenantID, &e.TenantName, &e.WorkspaceID, &e.WorkspaceName, &e.GrantID,
			&e.Actor, &e.ActorName, &e.Action, &e.Detail, &e.IP, &e.BreakGlass); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TenantAudit is what the customer sees: their tenant's entries, with a delayed (legal-hold)
// break-glass grant's entries hidden until its notice time.
func (d *DB) TenantAudit(ctx context.Context, tenantID string, limit int) ([]AuditEntry, error) {
	return d.audit(ctx, `WHERE a.tenant_id::text=$1 AND (g.id IS NULL OR g.notice_at<=now())
		ORDER BY a.id DESC LIMIT $2`, tenantID, limit)
}

// AllAudit is the operator view: every entry, optionally for one tenant.
func (d *DB) AllAudit(ctx context.Context, tenantID string, limit int) ([]AuditEntry, error) {
	return d.audit(ctx, `WHERE ($1='' OR a.tenant_id::text=$1) ORDER BY a.id DESC LIMIT $2`, tenantID, limit)
}
