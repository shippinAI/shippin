package orgs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/projects"
)

// Store is the org registry. Now comes from the caller (tests pin it); nothing is hardcoded.
type Store struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const orgCols = `id, slug, type, name, coalesce(nomen_org_id,'')`

func scanOrg(r pgx.Row) (*Org, error) {
	var o Org
	if err := r.Scan(&o.ID, &o.Slug, &o.Type, &o.Name, &o.NomenOrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}

// EnsureOperations creates the single Operations org from config (slug, name, Nomen org) or returns
// the existing one. The unique partial index makes a second Operations org impossible.
func (s *Store) EnsureOperations(ctx context.Context, slug, name, nomenOrg string) (*Org, error) {
	if !ValidSlug(slug) || name == "" {
		return nil, ErrBadInput
	}
	if o, err := s.Operations(ctx); err == nil {
		return o, nil
	}
	return scanOrg(s.Pool.QueryRow(ctx, `INSERT INTO orgs(id, slug, type, name, nomen_org_id)
		VALUES($1,$2,'operations',$3,nullif($4,'')) RETURNING `+orgCols, "org_"+projects.NewULID(), slug, name, nomenOrg))
}

// Operations returns the Operations org.
func (s *Store) Operations(ctx context.Context) (*Org, error) {
	return scanOrg(s.Pool.QueryRow(ctx, `SELECT `+orgCols+` FROM orgs WHERE type='operations'`))
}

// EnsureCustomer returns the customer org for a Nomen org, creating it on first sight.
func (s *Store) EnsureCustomer(ctx context.Context, nomenOrg, slug, name string) (*Org, error) {
	if nomenOrg == "" || !ValidSlug(slug) || name == "" {
		return nil, ErrBadInput
	}
	o, err := scanOrg(s.Pool.QueryRow(ctx, `SELECT `+orgCols+` FROM orgs WHERE nomen_org_id=$1`, nomenOrg))
	if err == nil || !errors.Is(err, ErrNotFound) {
		return o, err
	}
	return scanOrg(s.Pool.QueryRow(ctx, `INSERT INTO orgs(id, slug, type, name, nomen_org_id)
		VALUES($1,$2,'customer',$3,$4) ON CONFLICT (nomen_org_id) DO UPDATE SET name=orgs.name RETURNING `+orgCols,
		"org_"+projects.NewULID(), slug, name, nomenOrg))
}

// StaffRolesOf returns the caller's active staff roles (not revoked, not expired).
func (s *Store) StaffRolesOf(ctx context.Context, sub string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT role FROM staff_assignments WHERE user_sub=$1 AND revoked_at IS NULL
		AND (expires_at IS NULL OR expires_at > $2) ORDER BY role`, sub, s.now())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// GrantStaffRole adds or renews a staff role (bootstrap from config, or by an ops_admin).
func (s *Store) GrantStaffRole(ctx context.Context, sub, role, by string, expires *time.Time) error {
	if sub == "" || by == "" || !CanReadPlatform([]string{role}) {
		return ErrBadInput
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO staff_assignments(user_sub, role, granted_by, expires_at) VALUES($1,$2,$3,$4)
		ON CONFLICT (user_sub, role) DO UPDATE SET granted_by=$3, granted_at=now(), expires_at=$4, revoked_at=NULL`,
		sub, role, by, expires)
	return err
}

// AddFolder creates a folder under an org (optionally nested).
func (s *Store) AddFolder(ctx context.Context, orgID, parentID, name string) (string, error) {
	if !ValidOrgID(orgID) || name == "" || len(name) > 80 {
		return "", ErrBadInput
	}
	id := "fld_" + projects.NewULID()
	_, err := s.Pool.Exec(ctx, `INSERT INTO folders(id, org_id, parent_id, name) VALUES($1,$2,nullif($3,''),$4)`, id, orgID, parentID, name)
	return id, err
}
