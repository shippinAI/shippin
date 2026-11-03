package projects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Placements a project can run in. Converting between them is a data change plus a migration job.
const (
	PlacementContainer = "container-in-tenant-vm"
	PlacementVM        = "dedicated-vm"
)

// Limits are the project's resource caps (Incus limits.cpu, limits.memory, root size, limits.ingress/egress).
type Limits struct {
	CPU  string `json:"cpu,omitempty"`
	Mem  string `json:"mem,omitempty"`
	Disk string `json:"disk,omitempty"`
	Net  string `json:"net,omitempty"`
}

// OpProject is an operator-created project, adopting an existing devspace (workspace).
type OpProject struct {
	TenantID, Slug, OwnerSub, WorkspaceID, Placement, CostCenter, CreatedBy string
	SourcePath, SourceRemote                                                string // folder in the devspace; git remote
	RequireDevspaceOwner                                                    bool   // tenant adopt/import: OwnerSub must own the devspace
	Limits                                                                  Limits
}

// Created is what the operator create returns.
type Created struct {
	ID, PublicID, OrgID, EnvironmentID string
	BoundDevspace                      bool   // this project took the devspace (it had none)
	DevspaceOwner                      string // workspaces.created_by
}

var (
	ErrNoOrg            = errors.New("tenant has no Nomen org")
	ErrWorkspaceTaken   = errors.New("devspace is not this tenant's")
	ErrNotDevspaceOwner = errors.New("only the devspace's owner can add projects from it")
	ErrPlacement        = errors.New("placement must be container-in-tenant-vm or dedicated-vm")
)

// CreateOperator inserts the project (org from tenants.idp_org_id, slug unique per org), its dev
// environment, and binds the devspace, in one transaction.
func (s *Store) CreateOperator(ctx context.Context, in OpProject) (*Created, error) {
	if err := ValidSlug(in.Slug); err != nil {
		return nil, err
	}
	if in.Placement == "" {
		in.Placement = PlacementContainer
	}
	if in.Placement != PlacementContainer && in.Placement != PlacementVM {
		return nil, ErrPlacement
	}
	if len(in.CostCenter) > 64 {
		return nil, fmt.Errorf("cost_center is at most 64 chars")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	out := &Created{PublicID: NewPublicID()}
	if err := tx.QueryRow(ctx, `SELECT idp_org_id FROM tenants WHERE kind='customer' AND id=$1`, in.TenantID).Scan(&out.OrgID); err != nil || out.OrgID == "" {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoOrg
		}
		return nil, err
	}
	var boundary, bound *string
	err = tx.QueryRow(ctx, `SELECT boundary_id::text, project_id::text, created_by FROM workspaces WHERE id=$1 AND tenant_id=$2
	  AND state NOT IN ('deleted','failed') FOR UPDATE`, in.WorkspaceID, in.TenantID).Scan(&boundary, &bound, &out.DevspaceOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWorkspaceTaken
	} else if err != nil {
		return nil, err
	}
	if in.RequireDevspaceOwner && out.DevspaceOwner != in.OwnerSub {
		return nil, ErrNotDevspaceOwner
	}
	pid, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	lim, _ := json.Marshal(in.Limits)
	if _, err := tx.Exec(ctx, `INSERT INTO projects(id,tenant_id,boundary_id,name,state,created_by,public_id,org_id,slug,owner_sub,
	    placement,limits,cost_center,source_path,source_remote,source_workspace_id) VALUES($1,$2,$3,$4,'ready',$5,$6,$7,$4,$8,$9,$10,$11,NULLIF($12,''),$13,$14)`,
		pid, in.TenantID, boundary, in.Slug, in.CreatedBy, out.PublicID, out.OrgID, in.OwnerSub, in.Placement, lim, in.CostCenter,
		in.SourcePath, in.SourceRemote, in.WorkspaceID); err != nil {
		return nil, uniqueViolation(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO environments(id,project_id,tenant_id,name) VALUES($1,$2,$3,'dev')`, eid, pid, in.TenantID); err != nil {
		return nil, err
	}
	// The first project adopts the devspace; later folders in the same devspace share it.
	if _, err := tx.Exec(ctx, `UPDATE workspaces SET project_id=$1, environment_id=$2, updated_at=now() WHERE id=$3 AND project_id IS NULL`,
		pid, eid, in.WorkspaceID); err != nil {
		return nil, err
	}
	out.BoundDevspace = bound == nil
	out.ID, out.EnvironmentID = pid.String(), eid.String()
	return out, tx.Commit(ctx)
}

// UndoOperator removes a project made by CreateOperator when a later step (tuples) failed, so no
// project exists without its OpenFGA ownership.
func (s *Store) UndoOperator(ctx context.Context, tenant, project string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, q := range []string{
		`UPDATE workspaces SET project_id=NULL, environment_id=NULL WHERE tenant_id=$1 AND project_id=$2`,
		`DELETE FROM environments WHERE tenant_id=$1 AND project_id=$2`,
		`DELETE FROM projects WHERE tenant_id=$1 AND id=$2`,
	} {
		if _, err := tx.Exec(ctx, q, tenant, project); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
