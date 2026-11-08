package orgs

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"shippin.cloud/console/internal/projects"
)

// CreateLocalCustomer creates identity, membership, registry and the tenant in
// one transaction. Tenant insertion creates its default workspace. No resources
// are provisioned here, and the acting staff member receives no implicit access.
func (s *Store) CreateLocalCustomer(ctx context.Context, name, plan, owner, actor string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 || strings.ContainsFunc(name, unicode.IsControl) || uuid.Validate(owner) != nil || actor == "" {
		return "", ErrBadInput
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var found string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM identity_users WHERE id=$1 AND active FOR KEY SHARE`, owner).Scan(&found); err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM plans WHERE id=$1 AND active FOR SHARE`, plan).Scan(&found); err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrBadInput
		}
		return "", err
	}
	directory, tenant := uuid.NewString(), uuid.NewString()
	registry := "org_" + projects.NewULID()
	if _, err := tx.Exec(ctx, `INSERT INTO identity_orgs(id,name) VALUES($1,$2)`, directory, name); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan,status,kind) VALUES($1,$2,$3,$4,'draft','customer')`, tenant, directory, name, plan); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO orgs(id,slug,type,name,nomen_org_id,identity_org_id,tenant_id) VALUES($1,$2,'customer',$3,$4,$5,$6)`, registry, "customer-"+strings.ToLower(registry[4:]), name, directory, directory, tenant); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_members(org_id,user_id,role) VALUES($1,$2,'owner')`, directory, owner); err != nil {
		return "", err
	}
	detail, _ := json.Marshal(map[string]string{"tenant_id": tenant, "organization_id": registry, "owner_user_id": owner, "plan_id": plan})
	if _, err := tx.Exec(ctx, `INSERT INTO identity_audit(actor,action,detail) VALUES($1,'organization.created',$2)`, actor, detail); err != nil {
		return "", err
	}
	return tenant, tx.Commit(ctx)
}
