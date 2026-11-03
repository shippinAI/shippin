package orgs

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const grantCols = `id::text, staff_sub, org_id, coalesce(project_id,''), reason, granted_by, created_at, expires_at, revoked_at`

func scanGrant(r pgx.Row) (*SupportGrant, error) {
	var g SupportGrant
	if err := r.Scan(&g.ID, &g.StaffSub, &g.OrgID, &g.ProjectID, &g.Reason, &g.GrantedBy, &g.CreatedAt, &g.ExpiresAt, &g.RevokedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &g, nil
}

// GrantSupport gives one staff member time-limited access to one customer org (optionally one
// project). ttl must be > 0 and <= max (config). Operations itself is not a grant target.
func (s *Store) GrantSupport(ctx context.Context, staff, orgID, project, reason, by string, ttl, max time.Duration) (*SupportGrant, error) {
	reason = strings.TrimSpace(reason)
	if staff == "" || by == "" || !ValidOrgID(orgID) || len(reason) < 10 || len(reason) > 500 || ttl <= 0 {
		return nil, ErrBadInput
	}
	if ttl > max {
		return nil, ErrTooLong
	}
	var typ string
	if err := s.Pool.QueryRow(ctx, `SELECT type FROM orgs WHERE id=$1`, orgID).Scan(&typ); err != nil {
		return nil, ErrNotFound
	}
	if typ != TypeCustomer {
		return nil, ErrBadInput
	}
	now := s.now()
	return scanGrant(s.Pool.QueryRow(ctx, `INSERT INTO support_grants(id, staff_sub, org_id, project_id, reason, granted_by, created_at, expires_at)
		VALUES($1,$2,$3,nullif($4,''),$5,$6,$7,$8) RETURNING `+grantCols, uuid.NewString(), staff, orgID, project, reason, by, now, now.Add(ttl)))
}

// RevokeSupport ends a grant now (idempotent on an already revoked grant).
func (s *Store) RevokeSupport(ctx context.Context, id string) (*SupportGrant, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	return scanGrant(s.Pool.QueryRow(ctx, `UPDATE support_grants SET revoked_at=coalesce(revoked_at,$2) WHERE id=$1 RETURNING `+grantCols, id, s.now()))
}

// SupportGrants lists grants, newest first (all, for the audit view).
func (s *Store) SupportGrants(ctx context.Context, limit int) ([]SupportGrant, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+grantCols+` FROM support_grants ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SupportGrant{}
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// HasSupport reports whether staff currently holds an active grant on orgID.
func (s *Store) HasSupport(ctx context.Context, staff, orgID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM support_grants WHERE staff_sub=$1 AND org_id=$2
		AND revoked_at IS NULL AND expires_at > $3)`, staff, orgID, s.now()).Scan(&ok)
	return ok, err
}
