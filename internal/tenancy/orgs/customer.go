package orgs

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"net/mail"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"shippin.cloud/console/internal/tenancy/projects"
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
	tenant, err := createLocalCustomer(ctx, tx, name, plan, owner, actor)
	if err != nil {
		return "", err
	}
	return tenant, tx.Commit(ctx)
}

// Q is the database a customer is created on: a transaction, so its rows are kept or lost together.
type Q interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func createLocalCustomer(ctx context.Context, tx Q, name, plan, owner, actor string) (string, error) {
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
	return tenant, nil
}

// CustomerForEmail is the customer of the person with that email address, for placing something they
// already run under a customer when all that is known is who to write to. A person who owns a customer
// keeps it, the oldest when there are several. Anyone else gets a new draft customer, named name or
// after the address, on plan or the lowest active plan, and an account that is theirs to take over when
// they are invited. The person may be staff: a staff member is a customer like anyone else, and gets
// nothing from it but that membership. q is the caller's transaction.
func CustomerForEmail(ctx context.Context, q Q, email, name, plan, actor string) (tenant string, created bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, perr := mail.ParseAddress(email)
	if perr != nil || address.Address != email || len(email) > 254 || actor == "" {
		return "", false, ErrBadInput
	}
	var user string
	err = q.QueryRow(ctx, `SELECT id::text FROM identity_users WHERE email=$1`, email).Scan(&user)
	switch {
	case err == pgx.ErrNoRows:
		user = uuid.Must(uuid.NewV7()).String()
		if _, err := q.Exec(ctx, `INSERT INTO identity_users(id,email,email_verified,name,role) VALUES($1,$2,false,'','member')`, user, email); err != nil {
			return "", false, err
		}
	case err != nil:
		return "", false, err
	default:
		err = q.QueryRow(ctx, `SELECT t.id::text FROM tenants t JOIN orgs o ON o.tenant_id=t.id
		  JOIN identity_members m ON m.org_id=o.identity_org_id
		  WHERE m.user_id=$1 AND m.role='owner' AND o.type='customer' AND t.kind='customer' ORDER BY t.created_at, t.id LIMIT 1`, user).Scan(&tenant)
		if err == nil {
			return tenant, false, nil
		}
		if err != pgx.ErrNoRows {
			return "", false, err
		}
	}
	if strings.TrimSpace(name) == "" {
		name = email
	}
	if plan == "" {
		if err := q.QueryRow(ctx, `SELECT id FROM plans WHERE active ORDER BY rank, id LIMIT 1`).Scan(&plan); err != nil {
			if err == pgx.ErrNoRows {
				return "", false, ErrBadInput
			}
			return "", false, err
		}
	}
	tenant, err = createLocalCustomer(ctx, q, name, plan, user, actor)
	return tenant, err == nil, err
}
