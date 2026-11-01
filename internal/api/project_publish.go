package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/gh"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/projects"
)

// publishProject is "Create GitHub repo & sync" for an adopted folder with no remote: create a
// private repo named after the project in the tenant's org installation (or link an existing empty
// one, repo_id), then git init/commit/remote/push in the devspace through hostd's allowlisted git op.
func (a *API) publishProject(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok || !a.githubFor(w, t) {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Publishing projects needs the org owner or admin role.")
		return
	}
	var in struct {
		InstallationID int64 `json:"installation_id"`
		RepoID         int64 `json:"repo_id"` // 0 = create a new repository
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.InstallationID == 0 {
		auth.Problem(w, http.StatusBadRequest, "Pick a GitHub account.")
		return
	}
	src, err := (&projects.Store{Pool: a.db.Pool}).SourceOf(r.Context(), t.ID, r.PathValue("projectId"))
	if err != nil || src.Remote != "" {
		auth.Problem(w, http.StatusConflict, "Only a devspace folder without a GitHub remote can be published.")
		return
	}
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) {
		repo, status, msg := a.publishRepo(r.Context(), t.ID, in.InstallationID, in.RepoID, src.Slug)
		if repo == nil {
			auth.Problem(w, status, msg)
			return
		}
		if err := a.pushFolder(r.Context(), t.ID, src, in.InstallationID, repo); err != nil {
			auth.Problem(w, http.StatusBadGateway, "The repository "+repo.FullName+" exists but the first push failed; try again.")
			return
		}
		_ = a.store().AddAudit(r.Context(), db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: src.WorkspaceID,
			Actor: c.id.Sub, ActorName: c.id.Name, Action: "project.github.publish", Detail: src.Slug + " -> " + repo.FullName, IP: clientIP(r)})
		writeJSON(w, map[string]any{"repo": repo.FullName, "remote": "https://github.com/" + repo.FullName + ".git"})
	})
}

// publishRepo creates (repoID 0) or verifies the target repository of a tenant installation.
func (a *API) publishRepo(ctx context.Context, tenant string, inst, repoID int64, slug string) (*gh.Repo, int, string) {
	ins, err := a.db.Installations(ctx, tenant)
	i := slices.IndexFunc(ins, func(g db.GitHubInstallation) bool { return g.ID == inst && !g.Suspended })
	if err != nil || i < 0 {
		return nil, http.StatusNotFound, "That GitHub installation isn't connected to this tenant."
	}
	if repoID != 0 {
		repo, err := a.gh.Repo(ctx, inst, repoID, a.cfg.GitHub.MaxRepos)
		if err != nil || repo == nil {
			return nil, http.StatusNotFound, "Shippin can't see that repository."
		}
		return repo, 0, ""
	}
	repo, err := a.gh.CreateOrgRepo(ctx, inst, ins[i].AccountLogin, ins[i].AccountType, slug)
	switch {
	case errors.Is(err, gh.ErrUserAccount):
		return nil, http.StatusUnprocessableEntity, "GitHub lets an App create repositories only in organizations. Create an empty repository named " + slug + " on GitHub, then link it."
	case err != nil:
		return nil, http.StatusBadGateway, "GitHub didn't create the repository (does the App have Administration: write?)."
	}
	return repo, 0, ""
}

// pushFolder pushes the folder to repo and records the remote and the GitHub source.
func (a *API) pushFolder(ctx context.Context, tenant string, src *projects.Source, inst int64, repo *gh.Repo) error {
	tok, err := a.gh.PushToken(ctx, inst, repo.ID, true)
	if err != nil {
		return err
	}
	branch := repo.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	if err := a.host.Git(ctx, tenant, src.WorkspaceID, hostclient.GitOp{Op: "publish", Path: src.Path, Repo: repo.FullName,
		Branch: branch, GitToken: &hostclient.GitToken{Token: tok.Value, ExpiresAt: tok.ExpiresAt}}); err != nil {
		return err
	}
	if err := (&projects.Store{Pool: a.db.Pool}).SetRemote(ctx, tenant, src.ID, "https://github.com/"+repo.FullName+".git"); err != nil {
		return err
	}
	return a.db.SetSource(ctx, db.ProjectSource{ProjectID: src.ID, TenantID: tenant, Kind: "github", InstallationID: inst,
		RepoID: repo.ID, RepoFullName: repo.FullName, DefaultBranch: branch, Branch: branch})
}
