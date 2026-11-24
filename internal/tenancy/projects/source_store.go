package projects

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Source is where an adopted or imported project lives: its devspace folder and git remote.
type Source struct {
	ID, PublicID, Slug, Path, Remote, WorkspaceID string
}

// ErrNoSource: the project has no devspace folder (made before R-J1a, or blank).
var ErrNoSource = errors.New("project has no devspace folder")

// SourceOf loads a live project's folder, remote and devspace.
func (s *Store) SourceOf(ctx context.Context, tenant, project string) (*Source, error) {
	var o Source
	err := s.Pool.QueryRow(ctx, `SELECT id::text, coalesce(public_id,''), coalesce(slug,name), coalesce(source_path,''), source_remote,
	  coalesce(source_workspace_id::text,'') FROM projects WHERE tenant_id=$1 AND id::text=$2 AND state <> 'deleting'`, tenant, project).
		Scan(&o.ID, &o.PublicID, &o.Slug, &o.Path, &o.Remote, &o.WorkspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil && (o.Path == "" || o.WorkspaceID == "") {
		return nil, ErrNoSource
	}
	return &o, err
}

// SetRemote records the git remote after a publish; only a project without one is updated.
func (s *Store) SetRemote(ctx context.Context, tenant, project, remote string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE projects SET source_remote=$3, updated_at=now() WHERE tenant_id=$1 AND id::text=$2 AND source_remote=''`, tenant, project, remote)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrNoSource
	}
	return err
}
