package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Grant is a platform admin's access to a tenant (or one workspace) that the customer
// approved, or that break-glass activated. Rows are never deleted.
type Grant struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	TenantName    string     `json:"tenant_name"`
	WorkspaceID   string     `json:"workspace_id,omitempty"`
	WorkspaceName string     `json:"workspace_name,omitempty"`
	Kind          string     `json:"kind"`
	Requester     string     `json:"requester"`
	RequesterName string     `json:"requester_name"`
	Reason        string     `json:"reason"`
	Category      string     `json:"category,omitempty"`
	DurationS     int        `json:"duration_s"`
	Status        string     `json:"status"`
	DecidedBy     string     `json:"decided_by,omitempty"`
	DecidedName   string     `json:"decided_name,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	NoticeAt      *time.Time `json:"notice_at,omitempty"`
}

// Active reports an approved grant that hasn't expired at now.
func (g *Grant) Active(now time.Time) bool {
	return g.Status == "approved" && g.ExpiresAt != nil && now.Before(*g.ExpiresAt)
}

// Effective is the status as of now (an approved grant past expiry reads "expired" even
// before the sweeper closes it).
func (g *Grant) Effective(now time.Time) string {
	if g.Status == "approved" && !g.Active(now) {
		return "expired"
	}
	return g.Status
}

// Covers reports whether the grant gives access to workspace wsID ("" = tenant-level).
func (g *Grant) Covers(wsID string) bool { return g.WorkspaceID == "" || g.WorkspaceID == wsID }

const grantCols = `g.id::text, g.tenant_id::text, t.name, coalesce(g.workspace_id::text,''), coalesce(w.name,''),
	g.kind, g.requester, g.requester_name, g.reason, g.category, g.duration_s, g.status, g.decided_by,
	g.decided_name, g.created_at, g.decided_at, g.expires_at, g.ended_at, g.notice_at`

const grantFrom = ` FROM access_grants g JOIN tenants t ON t.id=g.tenant_id LEFT JOIN workspaces w ON w.id=g.workspace_id `

func scanGrants(rows pgx.Rows) ([]Grant, error) {
	defer rows.Close()
	out := []Grant{}
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.TenantID, &g.TenantName, &g.WorkspaceID, &g.WorkspaceName, &g.Kind,
			&g.Requester, &g.RequesterName, &g.Reason, &g.Category, &g.DurationS, &g.Status, &g.DecidedBy,
			&g.DecidedName, &g.CreatedAt, &g.DecidedAt, &g.ExpiresAt, &g.EndedAt, &g.NoticeAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (d *DB) grants(ctx context.Context, where string, args ...any) ([]Grant, error) {
	rows, err := d.Pool.Query(ctx, `SELECT `+grantCols+grantFrom+where, args...)
	if err != nil {
		return nil, err
	}
	return scanGrants(rows)
}

// ActiveGrants are the requester's approved, unexpired grants on live tenants.
func (d *DB) ActiveGrants(ctx context.Context, requester string) ([]Grant, error) {
	return d.grants(ctx, `WHERE g.requester=$1 AND g.status='approved' AND g.expires_at>now()
		AND t.status NOT IN ('deleting','deleted') ORDER BY g.created_at`, requester)
}

// TenantGrants lists a tenant's grants, newest first. customer hides withheld break-glass.
func (d *DB) TenantGrants(ctx context.Context, tenantID string, customer bool, limit int) ([]Grant, error) {
	q := `WHERE g.tenant_id::text=$1`
	if customer {
		q += ` AND g.notice_at IS NOT NULL AND g.notice_at<=now()`
	}
	return d.grants(ctx, q+` ORDER BY g.created_at DESC LIMIT $2`, tenantID, limit)
}

// AllGrants lists every grant, newest first (operators).
func (d *DB) AllGrants(ctx context.Context, limit int) ([]Grant, error) {
	return d.grants(ctx, `ORDER BY g.created_at DESC LIMIT $1`, limit)
}

// GrantByID returns one grant.
func (d *DB) GrantByID(ctx context.Context, id string) (*Grant, error) {
	gs, err := d.grants(ctx, `WHERE g.id::text=$1`, id)
	if err != nil || len(gs) == 0 {
		return nil, err
	}
	return &gs[0], nil
}

// OpenGrant reports a pending or active grant for the same requester and scope.
func (d *DB) OpenGrant(ctx context.Context, tenantID, wsID, requester string) (bool, error) {
	var n int
	err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM access_grants WHERE tenant_id::text=$1
		AND coalesce(workspace_id::text,'')=$2 AND requester=$3
		AND (status='pending' OR (status='approved' AND expires_at>now()))`, tenantID, wsID, requester).Scan(&n)
	return n > 0, err
}
