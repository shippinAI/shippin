package servers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server is one registry row as an org sees it. No secret or hash is ever part of it.
type Server struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"-"`
	Slug         string     `json:"slug"`
	Label        string     `json:"label"`
	Site         string     `json:"site"`
	State        State      `json:"state"`
	Estate       bool       `json:"estate"`
	Hostname     string     `json:"hostname"`
	AgentVersion string     `json:"agent_version"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	EnrolledAt   *time.Time `json:"enrolled_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SystemTenantID is the control plane's own tenant (migration 040). It owns the hosts added from the panel.
const SystemTenantID = "019a0000-0000-7000-8000-00000000c0de"

// Store is the registry; Clock derives stale, Now is the time seam for tests.
type Store struct {
	Pool  *pgxpool.Pool
	Clock Clock
	Now   func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const cols = `id, coalesce(tenant_id::text,''), slug, label, site, state, hostname, agent_version, last_seen_at, enrolled_at, created_at`

func (s *Store) scan(row pgx.Row) (*Server, error) {
	var v Server
	var st string
	if err := row.Scan(&v.ID, &v.TenantID, &v.Slug, &v.Label, &v.Site, &st, &v.Hostname, &v.AgentVersion,
		&v.LastSeenAt, &v.EnrolledAt, &v.CreatedAt); err != nil {
		return nil, err
	}
	v.Estate = v.TenantID == SystemTenantID
	v.State = s.Clock.Effective(State(st), v.LastSeenAt, s.now())
	return &v, nil
}

// List: the tenant's own servers, then platform hosts assigned to it (read-only), each by slug. A platform
// host with no assignment is none of a tenant's business, so it does not appear at all.
func (s *Store) List(ctx context.Context, tenant string) ([]Server, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+cols+` FROM servers WHERE tenant_id=$1 OR EXISTS (SELECT 1 FROM server_assignments a WHERE a.server_id=servers.id AND a.tenant_id=$1)
		ORDER BY tenant_id<>$1, slug`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Server{}
	for rows.Next() {
		v, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// Get returns a server the tenant can see (its own or one assigned to it).
func (s *Store) Get(ctx context.Context, tenant, id string) (*Server, error) {
	if uuid.Validate(id) != nil {
		return nil, ErrNotFound
	}
	v, err := s.scan(s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM servers WHERE id=$1 AND (tenant_id=$2 OR EXISTS (SELECT 1 FROM server_assignments a WHERE a.server_id=servers.id AND a.tenant_id=$2))`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// Count is the tenant's number of non-revoked servers (for the cap).
func (s *Store) Count(ctx context.Context, tenant string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM servers WHERE tenant_id=$1 AND state<>'revoked'`, tenant).Scan(&n)
	return n, err
}

// Create adds a pending server owned (and assigned) to the tenant.
func (s *Store) Create(ctx context.Context, tenant, slug, label, site, by string) (*Server, error) {
	id, _ := uuid.NewV7()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO servers(id, tenant_id, slug, label, site, state, created_by) VALUES($1,$2,$3,$4,$5,'pending',$6)`,
		id, tenant, slug, label, site, by)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, ErrSlugTaken
	} else if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO server_assignments(server_id, tenant_id, role, created_by) VALUES($1,$2,'owner',$3)`, id, tenant, by); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, tenant, id.String())
}
