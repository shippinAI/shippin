package api

import (
	"context"
	"errors"
	"fmt"

	"shippin.cloud/console/internal/authz"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/projects"
)

// ownedReq is one project create, shared by tenant Adopt and the operator route.
type ownedReq struct {
	TenantID, WorkspaceID, OwnerSub, By string
	OrganizationWorkspaceID             string
	RequireDevspaceOwner                bool   // tenant adopt/import (the operator route links only when owner_sub owns it)
	Slug                                string // preset (import picks it before cloning); else derived from Found.Path
	Found                               hostclient.Found
	Placement, CostCenter               string
	Limits                              projects.Limits
}

// ownedResult is what both routes return.
type ownedResult struct {
	Project  *projects.Created `json:"project"`
	Slug     string            `json:"slug"`
	RRFlow   string            `json:"rrflow"` // queued | attach-failed | unchanged | not-enabled
	Warnings []string          `json:"warnings,omitempty"`
}

var errNoTupleWriter = errors.New("OpenFGA tuple writer is not configured")

// createOwned is the one create path: row (+ slug from the folder/repo name, unique in the Nomen
// org) -> the ownership tuples in one OpenFGA write -> undo the row if that write fails (no
// local-only ownership; no org role is written) -> rrflow for a newly bound, owner-linked devspace (failure leaves a retryable attach).
func (a *API) createOwned(ctx context.Context, in ownedReq) (*ownedResult, error) {
	if a.n2.tw == nil {
		return nil, errNoTupleWriter
	}
	st := &projects.Store{Pool: a.db.Pool}
	slug := in.Slug
	if slug == "" {
		var err error
		if slug, err = a.freeSlug(ctx, in.TenantID, in.Found.Path); err != nil {
			return nil, err
		}
	}
	c, err := st.CreateOperator(ctx, projects.OpProject{TenantID: in.TenantID, Slug: slug, OwnerSub: in.OwnerSub,
		WorkspaceID: in.WorkspaceID, OrganizationWorkspaceID: in.OrganizationWorkspaceID, Placement: in.Placement, CostCenter: in.CostCenter, Limits: in.Limits,
		CreatedBy: in.By, SourcePath: in.Found.Path, SourceRemote: in.Found.Remote, RequireDevspaceOwner: in.RequireDevspaceOwner})
	if err != nil {
		return nil, err
	}
	chain := authz.ProjectChain{TenantID: in.TenantID, OrgID: c.OrgID, ProjectID: c.ID, DevspaceID: in.WorkspaceID,
		Namespace: c.PublicID, OwnerSub: in.OwnerSub,
		LinkDevspace: c.BoundDevspace && c.DevspaceOwner == in.OwnerSub}
	if err := a.n2.tw.Write(ctx, chain.Tuples()); err != nil {
		if uerr := st.UndoOperator(ctx, in.TenantID, c.ID); uerr != nil {
			return nil, fmt.Errorf("ownership tuples failed (%v) and undo failed: %w", err, uerr)
		}
		return nil, fmt.Errorf("ownership tuples: %w", err)
	}
	res := &ownedResult{Project: c, Slug: slug, RRFlow: "unchanged"}
	switch {
	case !chain.LinkDevspace:
	case a.svcs == nil || !a.cfg.Services.RRFlow.Allows(in.TenantID):
		res.RRFlow = "not-enabled" // rrflow rollout is per tenant (PB/PD); attach later from the devspace
	default:
		res.RRFlow = "queued"
		if _, _, err := a.queueRRFlow(ctx, in.TenantID, c.ID, in.WorkspaceID, in.By); err != nil {
			res.RRFlow, res.Warnings = "attach-failed", []string{"rrflow could not be attached yet; retry from the devspace."}
		}
	}
	return res, nil
}

// freeSlug derives the slug from a folder path or repo name, unique within the tenant's Nomen org.
func (a *API) freeSlug(ctx context.Context, tenant, src string) (string, error) {
	st := &projects.Store{Pool: a.db.Pool}
	return projects.UniqueSlug(ctx, projects.SlugFromSource(src),
		func(ctx context.Context, s string) (bool, error) { return st.SlugTaken(ctx, tenant, s) })
}

// pickFound re-runs discovery and returns the entry at path, so only a real devspace folder can be adopted.
func (a *API) pickFound(ctx context.Context, tenant, ws, path string) (hostclient.Found, error) {
	items, err := a.host.Discover(ctx, tenant, ws)
	if err != nil {
		return hostclient.Found{}, err
	}
	for _, f := range items {
		if f.Path == path {
			return f, nil
		}
	}
	return hostclient.Found{}, errNotFound
}

var errNotFound = errors.New("folder is not a suggested project in this devspace")
