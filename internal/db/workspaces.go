package db
import (
	"context"
	"time"
)

type Workspace struct {
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

func (d *DB) Workspaces(ctx context.Context, tenant string) ([]Workspace, error) {
	return d.queryWS(ctx, `WHERE tenant_id=$1 AND state<>'deleted' ORDER BY created_at`, tenant)
}

// WorkspaceByHost finds a live workspace by its default host or an attached domain.
func (d *DB) WorkspaceByHost(ctx context.Context, host string) (*Workspace, error) {
	ws, err := d.queryWS(ctx, `WHERE state<>'deleted' AND (host=$1 OR $1 = ANY(domains)) LIMIT 1`, host)
	if err != nil || len(ws) == 0 {
		return nil, err
	}
	return &ws[0], nil
}

func (d *DB) queryWS(ctx context.Context, where string, args ...any) ([]Workspace, error) {
	rows, err := d.Pool.Query(ctx, `SELECT `+wsCols+` FROM workspaces `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workspace{}
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.TenantID, &w.Name, &w.State, &w.SSHPort, &w.IPv4, &w.SSHKeys, &w.CreatedBy, &w.Error, &w.CreatedAt, &w.Host, &w.Domains, &w.Firewall,
			&w.Template, &w.Repo, &w.Size, &w.Progress, &w.Previews, &w.ProjectID, &w.EnvironmentID, &w.BoundaryID); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// CreateWorkspace inserts a row with the lowest free SSH port in [portMin, portMax]; a
// concurrent create that takes the same port is retried.
func (d *DB) CreateWorkspace(ctx context.Context, w *Workspace, portMin, portMax int) error {
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

// BeginWorkspaceOp moves a live workspace into state unless an operation is already running
// (provisioning/deleting/starting/stopping). It returns false when another request won.
func (d *DB) BeginWorkspaceOp(ctx context.Context, id, state string) (bool, error) {
	tag, err := d.Pool.Exec(ctx, `UPDATE workspaces SET state=$2, error=NULL, updated_at=now()
		WHERE id=$1 AND state NOT IN ('provisioning','deleting','deleted','starting','stopping')`, id, state)
	return err == nil && tag.RowsAffected() == 1, err
}

func (d *DB) SetWorkspaceState(ctx context.Context, id, state, ipv4, errMsg string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET state=$2, ipv4=coalesce(nullif($3,''),ipv4), error=nullif($4,''), updated_at=now() WHERE id=$1`,
		id, state, ipv4, errMsg)
	return err
}

func (d *DB) SetWorkspaceAccess(ctx context.Context, id string, host string, domains []string, fw []Rule) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET host=coalesce(nullif($2,''),host), domains=$3, firewall=$4, updated_at=now() WHERE id=$1`,
		id, host, domains, fw)
	return err
}

func (d *DB) SetWorkspaceKeys(ctx context.Context, id string, keys []string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET ssh_keys=$2, updated_at=now() WHERE id=$1`, id, keys)
	return err
}

func (d *DB) SetWorkspaceProgress(ctx context.Context, id, progress string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET progress=$2, updated_at=now() WHERE id=$1`, id, progress)
	return err
}

func (d *DB) SetWorkspacePreviews(ctx context.Context, id string, ps []Preview) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces SET previews=$2, updated_at=now() WHERE id=$1`, id, ps)
	return err
}
