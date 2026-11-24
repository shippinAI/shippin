package projects

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrWorkspaceInput = errors.New("workspace name must be 1–80 characters and description at most 500 characters")
var ErrWorkspaceOccupied = errors.New("the default workspace or a workspace containing projects cannot be removed")

type WorkspaceInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (in *WorkspaceInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 80 || utf8.RuneCountInString(in.Description) > 500 || strings.ContainsFunc(in.Name, unicode.IsControl) {
		return ErrWorkspaceInput
	}
	return nil
}

const workspaceCols = `SELECT w.id,w.name,w.description,w.is_default,w.created_at,w.updated_at,
 (SELECT count(*) FROM projects p WHERE p.workspace_id=w.id AND p.state<>'deleting') AS projects
 FROM shippin_workspaces w`

func (s *Store) OrganizationWorkspaces(ctx context.Context, tenant string) (json.RawMessage, error) {
	return s.list(ctx, workspaceCols+` WHERE w.tenant_id=$1 ORDER BY w.is_default DESC,w.created_at,w.id`, tenant)
}

func (s *Store) OrganizationWorkspace(ctx context.Context, tenant, id string) (json.RawMessage, error) {
	if uuid.Validate(id) != nil {
		return nil, ErrNotFound
	}
	return s.one(ctx, workspaceCols+` WHERE w.tenant_id=$1 AND w.id=$2`, tenant, id)
}

func (s *Store) CreateOrganizationWorkspace(ctx context.Context, tenant, actor string, in WorkspaceInput) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err := s.Pool.Exec(ctx, `INSERT INTO shippin_workspaces(id,tenant_id,name,description,created_by) VALUES($1,$2,$3,$4,$5)`, id, tenant, in.Name, in.Description, actor)
	return id, uniqueViolation(err)
}

func (s *Store) UpdateOrganizationWorkspace(ctx context.Context, tenant, id string, in WorkspaceInput) error {
	if uuid.Validate(id) != nil {
		return ErrNotFound
	}
	if err := in.validate(); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE shippin_workspaces SET name=$3,description=$4,updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, id, in.Name, in.Description)
	if err != nil {
		return uniqueViolation(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteOrganizationWorkspace(ctx context.Context, tenant, id string) error {
	if uuid.Validate(id) != nil {
		return ErrNotFound
	}
	// Keep even deleting project history reachable. FK protection also closes a
	// concurrent project-create race with this delete.
	tag, err := s.Pool.Exec(ctx, `DELETE FROM shippin_workspaces w WHERE tenant_id=$1 AND id=$2 AND NOT is_default
 AND NOT EXISTS(SELECT 1 FROM projects p WHERE p.workspace_id=w.id)`, tenant, id)
	if err != nil {
		var constraint *pgconn.PgError
		if errors.As(err, &constraint) && constraint.Code == "23503" {
			return ErrWorkspaceOccupied
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.OrganizationWorkspace(ctx, tenant, id); err != nil {
			return err
		}
		return ErrWorkspaceOccupied
	}
	return nil
}

func (s *Store) WorkspaceProjects(ctx context.Context, tenant, id string) (json.RawMessage, error) {
	if _, err := s.OrganizationWorkspace(ctx, tenant, id); err != nil {
		return nil, err
	}
	return s.list(ctx, projectCols+` WHERE p.tenant_id=$1 AND p.workspace_id=$2 AND p.state<>'deleting' ORDER BY p.created_at`, tenant, id)
}
