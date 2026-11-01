package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ProjectSource is where a project's code comes from (018_github.sql).
type ProjectSource struct {
	ProjectID      string `json:"project_id"`
	TenantID       string `json:"tenant_id"`
	Kind           string `json:"kind"`
	InstallationID int64  `json:"installation_id,omitempty"`
	RepoID         int64  `json:"repo_id,omitempty"`
	RepoFullName   string `json:"repo_full_name,omitempty"`
	DefaultBranch  string `json:"default_branch,omitempty"`
	Branch         string `json:"branch,omitempty"`
	Status         string `json:"status"`
}

// SetSource records (or replaces) a project's source.
func (d *DB) SetSource(ctx context.Context, s ProjectSource) error {
	var inst, repo, name any
	if s.Kind == "github" {
		inst, repo, name = s.InstallationID, s.RepoID, s.RepoFullName
	}
	_, err := d.Pool.Exec(ctx, `INSERT INTO project_sources(project_id, tenant_id, kind, installation_id, repo_id, repo_full_name,
	    default_branch, branch) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	  ON CONFLICT (project_id) DO UPDATE SET kind=EXCLUDED.kind, installation_id=EXCLUDED.installation_id,
	    repo_id=EXCLUDED.repo_id, repo_full_name=EXCLUDED.repo_full_name, default_branch=EXCLUDED.default_branch,
	    branch=EXCLUDED.branch, status='ok', updated_at=now()`,
		s.ProjectID, s.TenantID, s.Kind, inst, repo, name, s.DefaultBranch, s.Branch)
	return err
}

const sourceCols = `project_id::text, tenant_id::text, kind, coalesce(installation_id,0), coalesce(repo_id,0),
  coalesce(repo_full_name,''), default_branch, branch, status`

func scanSource(r pgx.Row) (ProjectSource, error) {
	var s ProjectSource
	return s, r.Scan(&s.ProjectID, &s.TenantID, &s.Kind, &s.InstallationID, &s.RepoID, &s.RepoFullName, &s.DefaultBranch, &s.Branch, &s.Status)
}

// SourceOf returns a project's source (nil when it has none).
func (d *DB) SourceOf(ctx context.Context, project string) (*ProjectSource, error) {
	s, err := scanSource(d.Pool.QueryRow(ctx, `SELECT `+sourceCols+` FROM project_sources WHERE project_id::text=$1`, project))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

// RevokeRepos marks sources of repos removed from an installation as revoked.
func (d *DB) RevokeRepos(ctx context.Context, inst int64, repoIDs []int64) error {
	_, err := d.Pool.Exec(ctx, `UPDATE project_sources SET status='revoked', updated_at=now()
	  WHERE installation_id=$1 AND repo_id = ANY($2)`, inst, repoIDs)
	return err
}

// RenameRepo follows a repository rename.
func (d *DB) RenameRepo(ctx context.Context, repoID int64, fullName string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE project_sources SET repo_full_name=$2, updated_at=now() WHERE repo_id=$1`, repoID, fullName)
	return err
}

// GitHubWorkspace is a running workspace of a GitHub-imported project (token refresh).
type GitHubWorkspace struct {
	TenantID, WorkspaceID  string
	InstallationID, RepoID int64
}

// RunningGitHubWorkspaces lists running workspaces whose project imports an ok GitHub source.
func (d *DB) RunningGitHubWorkspaces(ctx context.Context) ([]GitHubWorkspace, error) {
	rows, err := d.Pool.Query(ctx, `SELECT w.tenant_id::text, w.id::text, s.installation_id, s.repo_id
	  FROM workspaces w JOIN project_sources s ON s.project_id = w.project_id
	  JOIN github_installations g ON g.id = s.installation_id
	  WHERE w.state='running' AND s.kind='github' AND s.status='ok' AND g.deleted_at IS NULL AND g.suspended_at IS NULL`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (GitHubWorkspace, error) {
		var g GitHubWorkspace
		return g, r.Scan(&g.TenantID, &g.WorkspaceID, &g.InstallationID, &g.RepoID)
	})
}

// ProjectName returns a project's name ("" when it doesn't exist in tenant).
func (d *DB) ProjectName(ctx context.Context, tenant, project string) (string, error) {
	var n string
	err := d.Pool.QueryRow(ctx, `SELECT name FROM projects WHERE id::text=$1 AND tenant_id::text=$2`, project, tenant).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return n, err
}
