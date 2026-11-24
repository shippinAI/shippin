package api

import (
	"context"
	"shippin.cloud/console/internal/runtime/devspace"
	"shippin.cloud/console/internal/runtime/template"
	"time"

	"shippin.cloud/console/internal/infrastructure/hostd"
)

type wsSpec struct {
	Template string
	Repo     string
	Size     devspace.Size
	Previews []devspace.Preview
}

// workspaceSpec validates the create-flow choices. Returns a user-facing message on error.
func (a *API) workspaceSpec(ctx context.Context, tplID, repo, size string) (wsSpec, string) {
	sp := wsSpec{Repo: repo, Previews: []devspace.Preview{}}
	if repo != "" && hostd.ValidRepo(repo, a.cfg.Workspaces.RepoHosts) != nil {
		return sp, repoMsg(a.cfg.Workspaces)
	}
	if tplID == "" {
		if repo != "" {
			return sp, "Pick a template to clone a repository."
		}
		return sp, ""
	}
	tpl, err := template.On(a.db.Pool).Get(ctx, tplID)
	if err != nil || tpl == nil || !tpl.Enabled {
		return sp, "That template isn't available."
	}
	sp.Template = tpl.ID
	if sp.Repo == "" {
		sp.Repo = tpl.Repo
	}
	sp.Size = devspace.Size{CPU: tpl.CPU, MemoryGiB: tpl.MemoryGiB, DiskGiB: tpl.DiskGiB}
	if size != "" {
		found := false
		for _, z := range a.cfg.Workspaces.Sizes {
			if z.ID == size {
				sp.Size, found = devspace.Size{CPU: z.CPU, MemoryGiB: z.MemoryGiB, DiskGiB: z.DiskGiB}, true
			}
		}
		if !found {
			return sp, "Unknown size."
		}
	}
	for _, p := range tpl.Ports {
		sp.Previews = append(sp.Previews, devspace.Preview{Port: p})
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
				_ = devspace.On(a.db.Pool).SetProgress(context.Background(), w, "")
				return
			case <-tick.C:
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				if p := a.host.Progress(ctx, t, w); p != "" && p != last {
					last = p
					_ = devspace.On(a.db.Pool).SetProgress(ctx, w, p)
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
