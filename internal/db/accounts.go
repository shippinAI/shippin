package db

import (
	"context"

	"github.com/google/uuid"
)

// UpsertAccount records the IdP subject as an account (013_projects.sql) and returns its id.
// A non-empty email replaces the stored one; an empty one never blanks it.
func (d *DB) UpsertAccount(ctx context.Context, sub, email string) (string, error) {
	id, _ := uuid.NewV7()
	var out string
	err := d.Pool.QueryRow(ctx, `INSERT INTO accounts(id, idp_sub, email) VALUES($1,$2,$3)
	  ON CONFLICT (idp_sub) DO UPDATE SET email = CASE WHEN EXCLUDED.email <> '' THEN EXCLUDED.email ELSE accounts.email END
	  RETURNING id::text`, id, sub, email).Scan(&out)
	return out, err
}

// AddTenantOwner links an account as an owner of a tenant (idempotent).
func (d *DB) AddTenantOwner(ctx context.Context, tenant, account string) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO tenant_owners(tenant_id, account_id) VALUES($1,$2)
	  ON CONFLICT DO NOTHING`, tenant, account)
	return err
}

// TenantOwners lists the account ids owning a tenant.
func (d *DB) TenantOwners(ctx context.Context, tenant string) ([]string, error) {
	rows, err := d.Pool.Query(ctx, `SELECT account_id::text FROM tenant_owners WHERE tenant_id=$1 ORDER BY created_at`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
