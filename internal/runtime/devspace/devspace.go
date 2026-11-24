// Package devspace is a development space: the machine a person builds a project in (WS-01). This file
// is its type and its storage; the actions beside it are what the panel and an AI call.
package devspace

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store reads and writes development spaces.
type Store struct{ Pool *pgxpool.Pool }

// On is the store on one database.
func On(p *pgxpool.Pool) Store { return Store{Pool: p} }

type Devspace struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	Name          string    `json:"name"`
	State         string    `json:"state"`
	SSHPort       int       `json:"ssh_port"`
	IPv4          string    `json:"ipv4,omitempty"`
	SSHKeys       []string  `json:"-"`
	CreatedBy     string    `json:"created_by"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	Host          string    `json:"host"`
	Domains       []string  `json:"domains"`
	Firewall      []Rule    `json:"firewall"`
	Template      string    `json:"template_id,omitempty"`
	Repo          string    `json:"repo"`
	Size          Size      `json:"size"`
	Progress      string    `json:"progress,omitempty"`
	Previews      []Preview `json:"previews"`
	ProjectID     string    `json:"project_id,omitempty"`
	EnvironmentID string    `json:"environment_id,omitempty"`
	BoundaryID    string    `json:"-"` // M1 (013_projects.sql); boundary hidden from users
}

// Size: per-workspace resources (0 = plan default).
type Size struct {
	CPU       int `json:"cpu"`
	MemoryGiB int `json:"memory_gib"`
	DiskGiB   int `json:"disk_gib"`
}

// Preview is one published dev-server port: https://<port>--<host>; private = owner only.
type Preview struct {
	Port   int  `json:"port"`
	Public bool `json:"public"`
}

// Rule is an inbound firewall rule stored as JSON on the workspace.
type Rule struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Source   string `json:"source"`
}

const wsCols = `id::text, tenant_id::text, name, state, ssh_port, coalesce(ipv4,''), ssh_keys, created_by, coalesce(error,''), created_at, coalesce(host,''), domains, firewall,
	coalesce(template_id::text,''), repo, size, progress, previews,
	coalesce(project_id::text,''), coalesce(environment_id::text,''), coalesce(boundary_id::text,'')`

func (d Store) List(ctx context.Context, tenant string) ([]Devspace, error) {
	return d.query(ctx, `WHERE tenant_id=$1 AND state<>'deleted' ORDER BY created_at`, tenant)
}

// ByHost finds a live workspace by its default host or an attached domain.
func (d Store) ByHost(ctx context.Context, host string) (*Devspace, error) {
	ws, err := d.query(ctx, `WHERE state<>'deleted' AND (host=$1 OR $1 = ANY(domains)) LIMIT 1`, host)
	if err != nil || len(ws) == 0 {
		return nil, err
	}
	return &ws[0], nil
}

func (d Store) query(ctx context.Context, where string, args ...any) ([]Devspace, error) {
	rows, err := d.Pool.Query(ctx, `SELECT `+wsCols+` FROM workspaces `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Devspace{}
	for rows.Next() {
		var w Devspace
		if err := rows.Scan(&w.ID, &w.TenantID, &w.Name, &w.State, &w.SSHPort, &w.IPv4, &w.SSHKeys, &w.CreatedBy, &w.Error, &w.CreatedAt, &w.Host, &w.Domains, &w.Firewall,
			&w.Template, &w.Repo, &w.Size, &w.Progress, &w.Previews, &w.ProjectID, &w.EnvironmentID, &w.BoundaryID); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// Create inserts a row with the lowest free SSH port in [portMin, portMax]; a
// concurrent create that takes the same port is retried.
func (d Store) Create(ctx context.Context, w *Devspace, portMin, portMax int) error {
	if w.Previews == nil {
		w.Previews = []Preview{}
	}
	if w.SSHKeys == nil {
		w.SSHKeys = []string{} // column is NOT NULL
	}
	for try := 0; try < 5; try++ {
		used, err := d.livePorts(ctx)
		if err != nil {
			return err
		}
		port, err := NextPort(used, portMin, portMax)
		if err != nil {
			return err
		}
		err = d.Pool.QueryRow(ctx, `INSERT INTO workspaces(id, tenant_id, name, ssh_port, ssh_keys, created_by, host, template_id, repo, size, previews,
			project_id, environment_id, boundary_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,$9,$10,$11,nullif($12,'')::uuid,nullif($13,'')::uuid,nullif($14,'')::uuid)
			RETURNING ssh_port, created_at`,
			w.ID, w.TenantID, w.Name, port, w.SSHKeys, w.CreatedBy, w.Host, w.Template, w.Repo, w.Size, w.Previews,
			w.ProjectID, w.EnvironmentID, w.BoundaryID).Scan(&w.SSHPort, &w.CreatedAt)
		if clash, err := classify(err); !clash {
			return err
		}
	}
	return ErrNoPorts
}

func (d Store) SetState(ctx context.Context, id, state, ipv4, errMsg string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET state=$2, ipv4=coalesce(nullif($3,''),ipv4), error=nullif($4,''), updated_at=now() WHERE id=$1`,
		id, state, ipv4, errMsg)
	return err
}

func (d Store) SetAccess(ctx context.Context, id string, host string, domains []string, fw []Rule) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET host=coalesce(nullif($2,''),host), domains=$3, firewall=$4, updated_at=now() WHERE id=$1`,
		id, host, domains, fw)
	return err
}

func (d Store) SetKeys(ctx context.Context, id string, keys []string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET ssh_keys=$2, updated_at=now() WHERE id=$1`, id, keys)
	return err
}

func (d Store) SetProgress(ctx context.Context, id, progress string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET progress=$2, updated_at=now() WHERE id=$1`, id, progress)
	return err
}

func (d Store) SetPreviews(ctx context.Context, id string, ps []Preview) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET previews=$2, updated_at=now() WHERE id=$1`, id, ps)
	return err
}

// ByCreator lists the live development spaces a person created (to give them changed SSH keys).
func (d Store) ByCreator(ctx context.Context, sub string) ([]Devspace, error) {
	return d.query(ctx, `WHERE created_by=$1 AND state<>'deleted'`, sub)
}

// All lists every live development space across organizations, for staff.
func (d Store) All(ctx context.Context) ([]Devspace, error) {
	return d.query(ctx, `WHERE state<>'deleted' ORDER BY created_at`)
}

// ByID returns a live development space.
func (d Store) ByID(ctx context.Context, id string) (*Devspace, error) {
	ws, err := d.query(ctx, `WHERE id::text=$1 AND state<>'deleted'`, id)
	if err != nil || len(ws) == 0 {
		return nil, err
	}
	return &ws[0], nil
}

// ActionTarget also returns a deleted one, only so a repeated delete can be answered with its
// receipt. Lists and reads leave deleted ones out.
func (d Store) ActionTarget(ctx context.Context, id string) (*Devspace, error) {
	rows, err := d.query(ctx, `WHERE id::text=$1`, id)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}
