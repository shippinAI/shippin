package api

import (
	"context"
	"errors"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/gh"
	"shippin.cloud/console/internal/hostclient"
)

// runningDevspace is the caller's own first running devspace ("" if none); imports never land in
// someone else's devspace.
func (a *API) runningDevspace(ctx context.Context, tenant, sub string) string {
	ws, err := a.db.Workspaces(ctx, tenant)
	if err != nil {
		return ""
	}
	for _, w := range ws {
		if w.State == "running" && w.CreatedBy == sub {
			return w.ID
		}
	}
	return ""
}

// cloneAndOwn picks the slug, clones into ~/code/<slug> with a read token for that repo only,
// then runs createOwned (row + tuples + undo) and records the GitHub source.
func (a *API) cloneAndOwn(ctx context.Context, tenant, ws, sub string, inst int64, repo *gh.Repo, branch, organizationWorkspace string) (*ownedResult, error) {
	slug, err := a.freeSlug(ctx, tenant, repo.FullName)
	if err != nil {
		return nil, err
	}
	if a.n2.tw == nil {
		return nil, errNoTupleWriter // checked before the clone, so nothing lands in the devspace
	}
	tok, err := a.gh.PushToken(ctx, inst, repo.ID, false)
	if err != nil {
		return nil, err
	}
	path := "/home/dev/code/" + slug
	if err := a.host.Git(ctx, tenant, ws, hostclient.GitOp{Op: "clone", Path: path, Repo: repo.FullName, Branch: branch,
		GitToken: &hostclient.GitToken{Token: tok.Value, ExpiresAt: tok.ExpiresAt}}); err != nil {
		return nil, errors.Join(errors.New("clone failed"), err)
	}
	res, err := a.createOwned(ctx, ownedReq{TenantID: tenant, WorkspaceID: ws, OrganizationWorkspaceID: organizationWorkspace, OwnerSub: sub, By: sub, Slug: slug, RequireDevspaceOwner: true,
		Found: hostclient.Found{Name: slug, Path: path, Git: true, Remote: "https://github.com/" + repo.FullName + ".git"}})
	if err != nil {
		return nil, err // the clone stays in ~/code and shows up under Suggested, so a retry is one Adopt
	}
	if err := a.db.SetSource(ctx, db.ProjectSource{ProjectID: res.Project.ID, TenantID: tenant, Kind: "github", InstallationID: inst,
		RepoID: repo.ID, RepoFullName: repo.FullName, DefaultBranch: repo.DefaultBranch, Branch: branch}); err != nil {
		res.Warnings = append(res.Warnings, "GitHub link not saved; the remote is still set in the folder.")
	}
	return res, nil
}
