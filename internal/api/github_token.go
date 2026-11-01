package api

import (
	"context"
	"log"
	"time"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
)

// ghTimers spaces the GitHub reconciler work (token refresh, installation poll) on the leader.
type ghTimers struct{ refresh, poll time.Time }

// createReq is hostReq plus, for a GitHub-imported workspace, a fresh token for the first clone.
func (a *API) createReq(ctx context.Context, t *tenantView, w db.Workspace) hostclient.WorkspaceReq {
	req := a.hostReq(t, w)
	req.GitToken = a.gitTokenFor(ctx, w.ProjectID)
	return req
}

// gitTokenFor mints the 1-hour installation token for a project's GitHub repo, narrowed to that one
// repository and github.token_permissions (nil when the project isn't an ok GitHub import).
func (a *API) gitTokenFor(ctx context.Context, project string) *hostclient.GitToken {
	if a.gh == nil || project == "" {
		return nil
	}
	src, err := a.db.SourceOf(ctx, project)
	if err != nil || src == nil || src.Kind != "github" || src.Status != "ok" {
		return nil
	}
	return a.mint(ctx, src.InstallationID, src.RepoID)
}

// pushTokenAfterStart gives a (re)started imported workspace a fresh token: /run is tmpfs.
func (a *API) pushTokenAfterStart(ctx context.Context, tenant string, ws db.Workspace, act string) {
	if act != "start" && act != "restart" {
		return
	}
	a.pushConnectionsAfterStart(ctx, tenant, ws, act)
	if tok := a.gitTokenFor(ctx, ws.ProjectID); tok != nil {
		if err := a.host.PutGitToken(ctx, tenant, ws.ID, *tok); err != nil {
			log.Printf("github: token after %s %s: %v", act, ws.ID, err)
		}
	}
}

func (a *API) mint(ctx context.Context, inst, repo int64) *hostclient.GitToken {
	t, err := a.gh.InstallationToken(ctx, inst, []int64{repo}, a.cfg.GitHub.TokenPermissions)
	if err != nil {
		log.Printf("github: token for installation %d: %v", inst, err)
		return nil
	}
	return &hostclient.GitToken{Token: t.Value, ExpiresAt: t.ExpiresAt}
}

// githubMaintenance (reconciler): every refresh_minutes re-push tokens into running imported
// workspaces; every poll_minutes reconcile installations with GitHub (webhooks may not arrive).
func (a *API) githubMaintenance(ctx context.Context) {
	g := a.cfg.GitHub
	if a.gh == nil {
		return
	}
	now := time.Now()
	if now.Sub(a.ghTick.refresh) >= time.Duration(g.RefreshMinutes)*time.Minute {
		a.ghTick.refresh = now
		a.refreshTokens(ctx)
	}
	if now.Sub(a.ghTick.poll) >= time.Duration(g.PollMinutes)*time.Minute {
		a.ghTick.poll = now
		a.pollInstallations(ctx)
		_ = a.db.PruneDeliveries(ctx)
	}
}

func (a *API) refreshTokens(ctx context.Context) {
	ws, err := a.db.RunningGitHubWorkspaces(ctx)
	if err != nil {
		log.Printf("github: refresh: %v", err)
		return
	}
	for _, w := range ws {
		if tok := a.mint(ctx, w.InstallationID, w.RepoID); tok != nil {
			if err := a.host.PutGitToken(ctx, w.TenantID, w.WorkspaceID, *tok); err != nil {
				log.Printf("github: refresh %s: %v", w.WorkspaceID, err)
			}
		}
	}
}

// pollInstallations marks installations GitHub no longer lists as deleted, and syncs suspension.
func (a *API) pollInstallations(ctx context.Context) {
	live, err := a.gh.Installations(ctx)
	if err != nil {
		log.Printf("github: poll: %v", err)
		return
	}
	seen := map[int64]bool{}
	for _, in := range live {
		seen[in.ID] = in.SuspendedAt != nil
	}
	ids, err := a.db.AllInstallationIDs(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		suspended, ok := seen[id]
		state := map[bool]string{true: "suspended", false: "active"}[suspended]
		if !ok {
			state = "deleted"
		}
		if err := a.db.SetInstallationState(ctx, id, state); err != nil {
			log.Printf("github: poll %d: %v", id, err)
		}
		if state == "active" {
			a.syncRepos(ctx, id)
		}
	}
}

// syncRepos re-derives which imported projects of installation inst still have repo access.
func (a *API) syncRepos(ctx context.Context, inst int64) {
	rs, err := a.gh.Repos(ctx, inst, a.cfg.GitHub.MaxRepos)
	if err != nil || len(rs) >= a.cfg.GitHub.MaxRepos { // a capped listing can't prove a repo is gone
		return
	}
	ids := make([]int64, 0, len(rs))
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	if err := a.db.SyncRepoAccess(ctx, inst, ids); err != nil {
		log.Printf("github: repo access %d: %v", inst, err)
	}
}
