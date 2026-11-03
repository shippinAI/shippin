package db

import (
	"context"
	"time"
)

type Tenant struct {
	ID        string    `json:"id"`
	IdpOrgID  string    `json:"idp_org_id"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan_id"`
	Status    string    `json:"status"`
	SSHIP     string    `json:"ssh_ip,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

const tenantCols = `id::text, idp_org_id, name, plan, status, coalesce(ssh_ip,''), coalesce(error,''), created_at`

func scanTenants(rows interface {
	Next() bool
	Scan(...any) error
}) ([]Tenant, error) {
	out := []Tenant{}
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.IdpOrgID, &t.Name, &t.Plan, &t.Status, &t.SSHIP, &t.Error, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// TenantsByOrgs returns tenants mapped to the given Nomen org ids (all if orgs is nil).
func (d *DB) TenantsByOrgs(ctx context.Context, orgs []string) ([]Tenant, error) {
	q, args := `SELECT `+tenantCols+` FROM tenants WHERE idp_org_id = ANY($1) ORDER BY created_at`, []any{orgs}
	if orgs == nil {
		q, args = `SELECT `+tenantCols+` FROM tenants ORDER BY created_at`, nil
	}
	rows, err := d.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTenants(rows)
}

func (d *DB) CreateTenant(ctx context.Context, t Tenant) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id, idp_org_id, name, plan, status) VALUES($1,$2,$3,$4,'provisioning')`,
		t.ID, t.IdpOrgID, t.Name, t.Plan)
	return err
}

func (d *DB) TenantByOrg(ctx context.Context, org string) (*Tenant, error) {
	ts, err := d.TenantsByOrgs(ctx, []string{org})
	if err != nil || len(ts) == 0 {
		return nil, err
	}
	return &ts[0], nil
}

func (d *DB) SetTenantStatus(ctx context.Context, id, status, sshIP, errMsg string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE tenants SET status=$2, ssh_ip=nullif($3,''), error=nullif($4,''), updated_at=now() WHERE id=$1`,
		id, status, sshIP, errMsg)
	return err
}

// TenantsByIDs returns the live (not deleting/deleted) tenants with the given ids.
func (d *DB) TenantsByIDs(ctx context.Context, ids []string) ([]Tenant, error) {
	rows, err := d.Pool.Query(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id::text = ANY($1)
		AND status NOT IN ('deleting','deleted') ORDER BY created_at`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTenants(rows)
}
