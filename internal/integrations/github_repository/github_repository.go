// Package github_repository is an organization's GitHub repositories, as its installations of the
// Shippin GitHub App can reach them, and where each one belongs: the workspace, and optionally the
// project, its work is for. A repository's jobs get Shippin runners only after a manager assigns it
// and allows runners (RUN-01, docs/runners.md). Anyone in the organization reads them; managers change
// them. GitHub itself is the ports.GitHub host port.
package github_repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// Repository is one repository and its assignment. WorkspaceID is null while it is unassigned.
type Repository struct {
	ID             string     `json:"id"`
	InstallationID string     `json:"installation_id"`
	FullName       string     `json:"full_name"`
	Private        bool       `json:"private"`
	WorkspaceID    *string    `json:"workspace_id"`
	ProjectID      *string    `json:"project_id"`
	Runners        bool       `json:"runners"`
	UpdatedAt      *time.Time `json:"updated_at"`
}

// ByID names one repository from the route by GitHub's repository id.
type ByID struct {
	ID string `path:"githubRepositoryId" json:"-"`
}

func (b ByID) number() (int64, error) {
	n, err := strconv.ParseInt(b.ID, 10, 64)
	if err != nil || n <= 0 {
		return 0, mcpl.NotFound("No GitHub repository " + b.ID + ".")
	}
	return n, nil
}

// installations are the organization's live installations, suspended ones left out.
func installations(c *mcpl.Call) ([]int64, error) {
	rows, err := c.DB.Query(c.Ctx, `SELECT id FROM github_installations WHERE tenant_id=$1 AND deleted_at IS NULL AND suspended_at IS NULL ORDER BY created_at`, c.Tenant)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// reachable finds repository id through the organization's own installations only.
func reachable(ctx context.Context, g ports.GitHub, insts []int64, id int64) (ports.GitHubRepository, int64, error) {
	for _, inst := range insts {
		rs, err := g.Repositories(ctx, inst)
		if err != nil {
			return ports.GitHubRepository{}, 0, mcpl.Conflict("GitHub didn't list this organization's repositories just now. Try again.")
		}
		for _, r := range rs {
			if r.ID == id {
				return r, inst, nil
			}
		}
	}
	return ports.GitHubRepository{}, 0, mcpl.NotFound("None of this organization's GitHub connections can reach repository " + strconv.FormatInt(id, 10) + ".")
}

const cols = `id::text, installation_id::text, full_name, private, workspace_id::text, project_id::text, runners, updated_at`

func scan(row pgx.Row) (Repository, error) {
	var r Repository
	var at time.Time
	err := row.Scan(&r.ID, &r.InstallationID, &r.FullName, &r.Private, &r.WorkspaceID, &r.ProjectID, &r.Runners, &at)
	r.UpdatedAt = &at
	return r, err
}

func stored(c *mcpl.Call, id int64) (Repository, bool, error) {
	r, err := scan(c.DB.QueryRow(c.Ctx, `SELECT `+cols+` FROM shippin_github_repositories WHERE id=$1 AND tenant_id=$2`, id, c.Tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}
