package api

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"slices"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
	"shippin.cloud/console/internal/projects"
)

// projectNameRe is the legacy project name form (prompt-created projects).
var projectNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,38}[a-z0-9]$`)

type importIn struct {
	InstallationID int64  `json:"installation_id"`
	RepoID         int64  `json:"repo_id"`
	Branch         string `json:"branch"`
	WorkspaceID    string `json:"workspace_id"`
}

// githubImport clones a repository of THIS tenant's installation into the running devspace
// (~/code/<slug>, slug from the repo name, unique in the Nomen org) and then creates the project on
// the shared path (createOwned: row, 8 OpenFGA tuples, undo on failure). The IDOR guard: the
// installation must be bound to the tenant, and GitHub must list the repo for it.
func (a *API) githubImport(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok || !a.githubFor(w, t) {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Importing projects needs the org owner or admin role.")
		return
	}
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) { a.importRepo(w, r, c, t) })
}

func (a *API) importRepo(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	var in importIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil || in.InstallationID == 0 || in.RepoID == 0 {
		auth.Problem(w, http.StatusBadRequest, "Pick a repository.")
		return
	}
	if in.WorkspaceID != "" {
		if _, err := (&projects.Store{Pool: a.db.Pool}).OrganizationWorkspace(r.Context(), t.ID, in.WorkspaceID); err != nil {
			auth.Problem(w, http.StatusNotFound, "Workspace not found.")
			return
		}
	}
	ins, err := a.db.Installations(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "GitHub is unavailable right now.")
		return
	}
	if !slices.ContainsFunc(ins, func(g db.GitHubInstallation) bool { return g.ID == in.InstallationID && !g.Suspended }) {
		auth.Problem(w, http.StatusNotFound, "That GitHub installation isn't connected to this tenant.")
		return
	}
	repo, err := a.gh.Repo(r.Context(), in.InstallationID, in.RepoID, a.cfg.GitHub.MaxRepos)
	if err != nil || repo == nil {
		auth.Problem(w, http.StatusNotFound, "Shippin can't see that repository. Check the App's repository access on GitHub.")
		return
	}
	branch := in.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}
	if hostd.ValidGitHubRepo("github:"+repo.FullName+"@"+branch) != nil {
		auth.Problem(w, http.StatusBadRequest, "That branch name isn't supported.")
		return
	}
	ws := a.runningDevspace(r.Context(), t.ID, c.id.Sub)
	if ws == "" {
		auth.Problem(w, http.StatusConflict, "Start your devspace first; the repository is cloned into it.")
		return
	}
	res, err := a.cloneAndOwn(r.Context(), t.ID, ws, c.id.Sub, in.InstallationID, repo, branch, in.WorkspaceID)
	if err != nil {
		log.Printf("import %s: %v", repo.FullName, err)
		auth.Problem(w, http.StatusBadGateway, "The import could not complete. If cloning succeeded, the folder remains in your devspace for recovery.")
		return
	}
	_ = a.store().AddAudit(r.Context(), db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: ws, Actor: c.id.Sub,
		ActorName: c.id.Name, Action: "project.import", Detail: repo.FullName + "@" + branch + " -> " + res.Slug, IP: clientIP(r)})
	writeStatus(w, http.StatusCreated, map[string]any{"project_id": res.Project.ID, "slug": res.Slug, "repo": repo.FullName,
		"branch": branch, "rrflow": res.RRFlow, "warnings": res.Warnings})
}
