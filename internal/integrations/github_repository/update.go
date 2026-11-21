package github_repository

import (
	"errors"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// UpdateInput is where a repository belongs: a workspace of this organization, optionally one of that
// workspace's projects, and whether its jobs may ask for Shippin runners.
type UpdateInput struct {
	ByID
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   *string `json:"project_id,omitempty"`
	Runners     bool    `json:"runners"`
}

// Update is shippin.integrations.github_repository.update: PUT /v1/tenants/{tenantId}/integrations/github-repositories/{githubRepositoryId}.
// It assigns the repository the first time and moves it after that. The repository must be one the
// organization's own GitHub connections reach, checked with GitHub now.
var Update = mcpl.Def[UpdateInput, Repository]{
	Spec: mcpl.Spec{Summary: "Assign a GitHub repository to a workspace or project, and allow its jobs on Shippin runners.", Access: mcpl.Member, Perm: "manage"},
	Run: func(c *mcpl.Call, in UpdateInput) (Repository, error) {
		id, err := in.number()
		if err != nil {
			return Repository{}, err
		}
		var project any
		if in.ProjectID != nil && *in.ProjectID != "" {
			project = *in.ProjectID
		}
		var inWorkspace bool
		err = c.DB.QueryRow(c.Ctx, `SELECT $3::text IS NULL OR EXISTS (SELECT 1 FROM projects WHERE id::text=$3 AND tenant_id=$1 AND workspace_id::text=$2)
		  FROM shippin_workspaces WHERE id::text=$2 AND tenant_id=$1`, c.Tenant, in.WorkspaceID, project).Scan(&inWorkspace)
		if errors.Is(err, pgx.ErrNoRows) {
			return Repository{}, mcpl.Invalid("Pick one of this organization's workspaces.")
		}
		if err != nil {
			return Repository{}, err
		}
		if !inWorkspace {
			return Repository{}, mcpl.Invalid("That project is not in the chosen workspace.")
		}
		g, err := mcpl.Need[ports.GitHub](c)
		if err != nil {
			return Repository{}, err
		}
		insts, err := installations(c)
		if err != nil {
			return Repository{}, err
		}
		gr, inst, err := reachable(c.Ctx, g, insts, id)
		if err != nil {
			return Repository{}, err
		}
		_, err = c.DB.Exec(c.Ctx, `INSERT INTO shippin_github_repositories(id, tenant_id, installation_id, full_name, private, workspace_id, project_id, runners, updated_by)
		  VALUES($1, $2, $3, $4, $5, $6::uuid, $7::uuid, $8, $9)
		  ON CONFLICT (id) DO UPDATE SET installation_id=EXCLUDED.installation_id, full_name=EXCLUDED.full_name, private=EXCLUDED.private,
		    workspace_id=EXCLUDED.workspace_id, project_id=EXCLUDED.project_id, runners=EXCLUDED.runners, updated_by=EXCLUDED.updated_by, updated_at=now()
		  WHERE shippin_github_repositories.tenant_id = EXCLUDED.tenant_id`,
			id, c.Tenant, inst, gr.FullName, gr.Private, in.WorkspaceID, project, in.Runners, c.Actor)
		if err != nil {
			return Repository{}, err
		}
		r, ok, err := stored(c, id)
		if err == nil && !ok {
			// Another organization holds this repository's assignment, through its own connection.
			return r, mcpl.Conflict("Another organization has already assigned this repository.")
		}
		return r, err
	},
}
