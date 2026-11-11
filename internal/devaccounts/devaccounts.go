// Package devaccounts keeps throwaway sign-ins for developing and reviewing the panel. They exist only on
// an installation that lists them in SHIPPIN_DEV_ACCOUNTS; nothing about them is in code or on by default.
// A staff account holds every staff role except Owner. A customer account owns its own organization and
// has no staff access. The installation owner's account is never touched.
package devaccounts

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/orgs"
)

// Kinds of development account.
const (
	Staff    = "staff"
	Customer = "customer"
)

// Account is one development sign-in.
type Account struct {
	Email string
	Kind  string
}

// Parse reads "email=kind,email=kind".
func Parse(raw string) ([]Account, error) {
	var out []Account
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		email, kind, ok := strings.Cut(part, "=")
		email = strings.ToLower(strings.TrimSpace(email))
		kind = strings.TrimSpace(kind)
		if a, err := mail.ParseAddress(email); !ok || err != nil || a.Address != email {
			return nil, fmt.Errorf("devaccounts: %q is not email=kind", part)
		}
		if kind != Staff && kind != Customer {
			return nil, fmt.Errorf("devaccounts: %s must be %s or %s", email, Staff, Customer)
		}
		if seen[email] {
			return nil, fmt.Errorf("devaccounts: %s is listed twice", email)
		}
		seen[email] = true
		out = append(out, Account{Email: email, Kind: kind})
	}
	if len(out) == 0 {
		return nil, errors.New("devaccounts: no accounts listed")
	}
	return out, nil
}

// ErrOwner means a listed address belongs to the installation owner, which is never changed here.
var ErrOwner = errors.New("devaccounts: that address is the installation owner")

// Ensure makes each account exist with the given password and its kind's access. It runs on every start,
// so the accounts always match the list.
func Ensure(ctx context.Context, pool *pgxpool.Pool, hash func(string) (string, error), accounts []Account, password string) error {
	encoded, err := hash(password)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if err := ensure(ctx, pool, a, encoded); err != nil {
			return fmt.Errorf("%s: %w", a.Email, err)
		}
	}
	return nil
}

func ensure(ctx context.Context, pool *pgxpool.Pool, a Account, encoded string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id, role string
	err = tx.QueryRow(ctx, `SELECT id::text, role FROM identity_users WHERE email=$1`, a.Email).Scan(&id, &role)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id = uuid.Must(uuid.NewV7()).String()
		name := map[string]string{Staff: "Development admin", Customer: "Development user"}[a.Kind]
		if _, err := tx.Exec(ctx, `INSERT INTO identity_users (id, email, email_verified, name, role) VALUES ($1,$2,true,$3,'member')`, id, a.Email, name); err != nil {
			return err
		}
	case err != nil:
		return err
	case role == "owner":
		return ErrOwner
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_users SET password_hash=$2, email_verified=true, active=true WHERE id=$1`, id, encoded); err != nil {
		return err
	}
	switch a.Kind {
	case Staff:
		var internal bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM orgs WHERE type='operations' AND identity_org_id IS NOT NULL)`).Scan(&internal); err != nil {
			return err
		}
		if !internal {
			return errors.New("the installation has no owner yet; finish setup and restart")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO local_staff_roles (user_id, roles)
		  SELECT $1, coalesce(array_agg(key ORDER BY key), '{}') FROM staff_roles WHERE key <> 'staff.owner'
		  ON CONFLICT (user_id) DO UPDATE SET roles = EXCLUDED.roles, updated_at = now()`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity_members (org_id, user_id, role)
		  SELECT identity_org_id, $1, 'member' FROM orgs WHERE type='operations' AND identity_org_id IS NOT NULL ON CONFLICT DO NOTHING`, id); err != nil {
			return err
		}
	case Customer:
		if _, err := tx.Exec(ctx, `DELETE FROM local_staff_roles WHERE user_id=$1`, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_audit(actor,action,detail) VALUES('devaccounts','dev_account.ensured',jsonb_build_object('email',$1::text,'kind',$2::text))`, a.Email, a.Kind); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if a.Kind == Customer {
		return ensureOrganization(ctx, pool, id)
	}
	return nil
}

// ensureOrganization gives a customer account an organization of its own, once.
func ensureOrganization(ctx context.Context, pool *pgxpool.Pool, user string) error {
	var has bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity_members m JOIN orgs o ON o.identity_org_id=m.org_id
	  WHERE o.type='customer' AND m.user_id=$1)`, user).Scan(&has); err != nil || has {
		return err
	}
	var plan string
	if err := pool.QueryRow(ctx, `SELECT id FROM plans WHERE active ORDER BY rank, id LIMIT 1`).Scan(&plan); err != nil {
		return fmt.Errorf("no active plan for its organization: %w", err)
	}
	_, err := (&orgs.Store{Pool: pool}).CreateLocalCustomer(ctx, "Development organization", plan, user, "devaccounts")
	return err
}
