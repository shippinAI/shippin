package orgs

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"shippin.cloud/console/internal/projects"
)

// ConfigureInternal binds the existing resource owner and operations registry
// to local identity in the setup transaction, never via customer memberships.
func ConfigureInternal(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	var tenant string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM tenants WHERE kind='system' FOR UPDATE`).Scan(&tenant); err != nil {
		return "", err
	}
	var id string
	var directory *string
	err := tx.QueryRow(ctx, `SELECT id,identity_org_id::text FROM orgs WHERE type='operations' FOR UPDATE`).Scan(&id, &directory)
	if errors.Is(err, pgx.ErrNoRows) {
		id = "org_" + projects.NewULID()
		_, err = tx.Exec(ctx, `INSERT INTO orgs(id,slug,type,name,tenant_id) VALUES($1,$2,'operations',$3,$4)`, id, "operations-"+strings.ToLower(id[len(id)-6:]), name, tenant)
	}
	if err != nil {
		return "", err
	}
	if directory == nil {
		value := uuid.NewString()
		directory = &value
		if _, err := tx.Exec(ctx, `INSERT INTO identity_orgs(id,name) VALUES($1,$2)`, value, name); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE orgs SET name=$2,identity_org_id=$3,tenant_id=$4 WHERE id=$1`, id, name, *directory, tenant); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_orgs SET name=$2 WHERE id=$1`, *directory, name); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE tenants SET name=$2 WHERE id=$1`, tenant, name); err != nil {
		return "", err
	}
	return *directory, nil
}
