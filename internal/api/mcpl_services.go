package api

import (
	"context"
	"errors"
	"fmt"
	"log"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/settings"
)

// provideServices hands actions and reactions what they Need besides the database. Each entity
// provides its own (actions.Providers, generated from `var Services = mcpl.Provider(...)` in its
// folder); the panel adds only the host ports of internal/ports, which need its keys or its hosts.
// Nothing here names an entity, so adding one never changes this file.
func (a *API) provideServices(s mcpl.Services) {
	for _, p := range actions.Providers {
		p(s)
	}
	mcpl.Provide[ports.Workspaces](s, workspacePort{a})
	mcpl.Provide[ports.Machines](s, machinePort{a})
	// The GitHub App can be turned on and off in the panel, so the port is always there and finds the
	// App on each call; while it is off, Need answers as if it weren't provided.
	mcpl.Provide[ports.GitHub](s, githubPort{a})
	// ClickHouse can be changed or turned off in the panel, so the port reads it on each call.
	mcpl.Provide[ports.Metrics](s, metricsPort{a})
	// Create the sealer only when actions are first requested. A missing key means secret-bearing
	// actions fail closed through mcpl.Need.
	if seal, err := a.sealer(); err == nil {
		mcpl.Provide[ports.Sealer](s, seal)
	}
	// The settings and the secret store their keys go to (docs/settings.md).
	if a.settings != nil {
		mcpl.Provide[*settings.Store](s, a.settings)
	}
	if st := a.secretStore(); st != nil {
		mcpl.Provide[ports.Secrets](s, st)
	}
}

// mountReactions runs every generated reaction on the events it follows, as the event's actor in the
// event's organization, after the change is committed.
func (a *API) mountReactions() {
	for _, r := range actions.Reactions {
		for _, on := range r.On {
			a.events.On(on, func(ctx context.Context, e mcpl.Event) {
				c := &mcpl.Call{Ctx: ctx, Actor: e.Actor, Via: "reaction", Tenant: e.Tenant, Services: a.actionServices(), Publish: a.events.Publish}
				if a.db != nil {
					c.Pool = a.db.Pool
				}
				if err := r.Run(c, e); err != nil {
					log.Printf("reaction %s on %s: %v", r.ID, e.ID, err)
				}
			})
		}
	}
}

// workspacePort is ports.Workspaces on this panel's database and hosts.
type workspacePort struct{ a *API }

// GiveKeys stores the keys on each of the person's workspaces and applies them to the running ones.
// One workspace failing does not stop the others; every failure is returned together.
func (p workspacePort) GiveKeys(ctx context.Context, sub string, keys []string) error {
	a := p.a
	wss, err := a.db.WorkspacesByCreator(ctx, sub)
	if err != nil {
		return err
	}
	var errs []error
	for _, ws := range wss {
		if err := a.db.SetWorkspaceKeys(ctx, ws.ID, keys); err != nil {
			errs = append(errs, fmt.Errorf("workspace %s: %w", ws.ID, err))
			continue
		}
		if ws.State == "running" {
			ws.SSHKeys = keys
			t := tenantView{Tenant: db.Tenant{ID: ws.TenantID}}
			if err := a.host.Action(ctx, ws.TenantID, ws.ID, "keys", a.hostReq(&t, ws)); err != nil {
				errs = append(errs, fmt.Errorf("workspace %s: stored, not applied until it restarts: %w", ws.ID, err))
			}
		}
	}
	return errors.Join(errs...)
}

// githubPort is ports.GitHub on this panel's GitHub App, as the github section has it now.
type githubPort struct{ a *API }

// Off reports whether the GitHub App is off (mcpl.Switchable).
func (p githubPort) Off(ctx context.Context) bool {
	app, _ := p.a.github(ctx)
	return app == nil
}

func (p githubPort) Repositories(ctx context.Context, installation int64) ([]ports.GitHubRepository, error) {
	app, g, err := p.a.githubOn(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := app.Repos(ctx, installation, g.MaxRepos)
	out := make([]ports.GitHubRepository, 0, len(rs))
	for _, r := range rs {
		out = append(out, ports.GitHubRepository{ID: r.ID, FullName: r.FullName, Private: r.Private})
	}
	return out, err
}
