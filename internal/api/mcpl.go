package api

import (
	"fmt"
	"net/http"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl"
)

// MCPL (docs/mcpl.md). Actions are files under internal/<domain>/<entity>/<verb>.go; cmd/mcplgen derives
// each one's ID and route from where it is and writes internal/actions. Nothing here lists actions by hand.

// actionRoutes mounts every generated action. Staff actions go through the same origin check, sign-in
// and permission gate as every other operator route; the action itself does the rest the same way.
func (a *API) actionRoutes() []opRoute {
	var out []opRoute
	for _, g := range actions.All {
		if g.Access != mcpl.Staff {
			panic(fmt.Sprintf("mcpl: %s is %s; only staff actions are mounted so far", g.ID, g.Access))
		}
		out = append(out, opRoute{g.Route, g.Perm, func(w http.ResponseWriter, r *http.Request, c *caller) {
			call := &mcpl.Call{Ctx: r.Context(), Pool: a.db.Pool, Actor: c.id.Sub,
				Perms: func(perm string) bool { ok, _ := a.decide(r.Context(), c, perm); return ok },
				Audit: func(action, detail string) error {
					return a.store().AddAudit(r.Context(), teamEntry(r, c, action, detail))
				}}
			g.Serve(call, w, r)
		}})
	}
	return out
}

func describedActions() []mcpl.Action {
	out := make([]mcpl.Action, 0, len(actions.All))
	for _, g := range actions.All {
		out = append(out, g.Action)
	}
	return out
}

// mcplPublic answers anyone: the contract version, the domains and the actions that need no session.
func (a *API) mcplPublic(w http.ResponseWriter, _ *http.Request) {
	public := []mcpl.Action{}
	for _, x := range describedActions() {
		if x.Access == mcpl.Public {
			public = append(public, x)
		}
	}
	writeJSON(w, map[string]any{"mcpl": mcpl.Version, "domains": mcpl.Domains, "actions": public})
}

// opMCPL lists every action for staff, marking the ones this caller may run.
func (a *API) opMCPL(w http.ResponseWriter, r *http.Request, c *caller) {
	type row struct {
		mcpl.Action
		Allowed bool `json:"allowed"`
	}
	rows := []row{}
	for _, x := range describedActions() {
		allowed := x.Access != mcpl.Staff
		if x.Access == mcpl.Staff {
			allowed, _ = a.decide(r.Context(), c, x.Perm)
		}
		rows = append(rows, row{x, allowed})
	}
	writeJSON(w, map[string]any{"mcpl": mcpl.Version, "domains": mcpl.Domains, "actions": rows})
}

func (a *API) mcplRoutes() []opRoute {
	return append([]opRoute{{"GET /v1/operator/mcpl", permPanel, a.opMCPL}}, a.actionRoutes()...)
}
