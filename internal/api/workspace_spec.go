package api

import (
	"context"
	"time"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
)

type wsSpec struct {
	Template string
	Repo     string
	Size     db.Size
	Previews []db.Preview
}

// workspaceSpec validates the create-flow choices. Returns a user-facing message on error.
func (a *API) workspaceSpec(ctx context.Context, tplID, repo, size string) (wsSpec, string) {
	sp := wsSpec{Repo: repo, Previews: []db.Preview{}}
	if repo != "" && hostd.ValidRepo(repo, a.cfg.Workspaces.RepoHosts) != nil {
		return sp, repoMsg(a.cfg.Workspaces)
	}
	if tplID == "" {
		if repo != "" {
			return sp, "Pick a template to clone a repository."
		}
		return sp, ""
	}
	tpl, err := a.db.Template(ctx, tplID)
	if err != nil || tpl == nil || !tpl.Enabled {
		return sp, "That template isn't available."
	}
	sp.Template = tpl.ID
	if sp.Repo == "" {
		sp.Repo = tpl.Repo
	}
	sp.Size = db.Size{CPU: tpl.CPU, MemoryGiB: tpl.MemoryGiB, DiskGiB: tpl.DiskGiB}
	if size != "" {
		found := false
		for _, z := range a.cfg.Workspaces.Sizes {
			if z.ID == size {
				sp.Size, found = db.Size{CPU: z.CPU, MemoryGiB: z.MemoryGiB, DiskGiB: z.DiskGiB}, true
			}
		}
		if !found {
			return sp, "Unknown size."
		}
	}
	for _, p := range tpl.Ports {
		sp.Previews = append(sp.Previews, db.Preview{Port: p})
	}
	return sp, ""
}

// trackProgress copies hostd's current provisioning step into the DB every second until stopped.
func (a *API) trackProgress(t, w string, also func(string)) func() {
	done := make(chan struct{})
	go func() {
		last := ""
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				_ = a.db.SetWorkspaceProgress(context.Background(), w, "")
				return
			case <-tick.C:
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				if p := a.host.Progress(ctx, t, w); p != "" && p != last {
					last = p
					_ = a.db.SetWorkspaceProgress(ctx, w, p)
					if also != nil {
						also(p)
					}
				}
				cancel()
			}
		}
	}()
	return func() { close(done) }
}
