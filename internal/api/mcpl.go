package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mcpl"
)

// MCPL (docs/mcpl.md). Actions are files under internal/<domain>/<entity>/<verb>.go; cmd/mcplgen derives
// each one's ID and route from where it is and writes internal/actions. Nothing here lists actions by hand.
// Each audience goes through the same gate as the routes written before actions: staff through the
// operator gate, members through the organization they belong to, everyone else through their session.

// actionRoutes are the staff actions, mounted with the operator routes behind the origin check, sign-in
// and permission gate.
func (a *API) actionRoutes() []opRoute {
	var out []opRoute
	for _, g := range actions.All {
		if g.Access != mcpl.Staff {
			continue
		}
		out = append(out, opRoute{g.Route, g.Perm, func(w http.ResponseWriter, r *http.Request, c *caller) {
			call := a.newCall(r, c)
			call.Perms = func(perm string) bool { ok, _ := a.decide(r.Context(), c, perm); return ok }
			g.Serve(call, w, r)
		}})
	}
	return out
}

// mountActions mounts the public, signed-in and member actions.
func (a *API) mountActions(mux Router) {
	for _, g := range actions.All {
		switch g.Access {
		case mcpl.Staff:
			// mounted with the operator routes
		case mcpl.Public:
			mux.HandleFunc(g.Route, func(w http.ResponseWriter, r *http.Request) { g.Serve(a.newCall(r, nil), w, r) })
		case mcpl.SignedIn:
			mux.HandleFunc(g.Route, a.flow.Require(func(w http.ResponseWriter, r *http.Request) {
				c, ok := a.caller(w, r)
				if !ok {
					return
				}
				g.Serve(a.newCall(r, c), w, r)
			}))
		case mcpl.Member:
			mux.HandleFunc(g.Route, a.flow.Require(func(w http.ResponseWriter, r *http.Request) {
				c, t, ok := a.tenantCtx(w, r)
				if !ok {
					return
				}
				if !memberMay(c, t, g.Perm) {
					auth.Problem(w, http.StatusForbidden, memberRefusal(g.Perm))
					return
				}
				call := a.newCall(r, c, func(e *db.AuditEntry) {
					e.TenantID = t.ID
					if g := t.grantFor(""); !t.member() && g != nil {
						e.GrantID, e.BreakGlass = g.ID, g.Kind == "breakglass"
					}
				})
				call.Tenant = t.ID
				call.Perms = func(role string) bool { return memberMay(c, t, role) }
				g.Serve(call, w, r)
			}))
		default:
			panic(fmt.Sprintf("mcpl: %s has unknown access %q", g.ID, g.Access))
		}
	}
	a.transport(mux, "GET /v1/me/mcpl/events", Stream, a.flow.Require(a.mcplEvents))
}

// newCall is one run of an action for c (nil for a public action): who it runs as, where its audit
// entry goes, and where its event is published. mark adds what the route knows to the entry.
func (a *API) newCall(r *http.Request, c *caller, mark ...func(*db.AuditEntry)) *mcpl.Call {
	call := &mcpl.Call{Ctx: r.Context(), Services: a.actionServices(), Publish: a.events.Publish}
	if a.db != nil {
		call.Pool = a.db.Pool
	}
	entry := db.AuditEntry{Actor: "anonymous", ActorName: "Someone without a session", IP: clientIP(r)}
	if c != nil {
		call.Actor, call.Via = c.id.Sub, c.method
		if auth.ViaAccessKey(r.Context()) {
			call.Via = "access_key"
		}
		entry = teamEntry(r, c, "", "")
	}
	call.Audit = func(q mcpl.DB, action, detail string) error {
		e := entry
		e.Action, e.Detail = action, detail
		for _, m := range mark {
			m(&e)
		}
		if q != nil && a.staffDB == nil {
			return db.AddAuditOn(r.Context(), q, e)
		}
		return a.store().AddAudit(r.Context(), e)
	}
	return call
}

// services are what actions Need besides the database. Each is provided under the interface its
// action package declares.
func (a *API) actionServices() mcpl.Services {
	a.actionSvcsOnce.Do(func() {
		a.actionSvcs = mcpl.Services{}
		a.provideServices(a.actionSvcs)
	})
	return a.actionSvcs
}

// memberMay answers a member action's role: "" anyone who sees the organization (a member, or staff
// holding an access grant), "manage" a manager, "decide" an owner or admin.
func memberMay(c *caller, t *tenantView, role string) bool {
	switch role {
	case "":
		return true
	case "manage":
		return c.canManage(t)
	case "decide":
		return c.canDecide(t)
	}
	return false
}

func memberRefusal(role string) string {
	if role == "decide" {
		return "Only the organization's owners and admins can do that."
	}
	return "Your role in this organization doesn't include that."
}

// mcplEvents streams the events of actions the caller could have run: their own, their organizations'
// at their role, and staff events their permissions cover. An AI client follows it to know when to act.
// A stream ends after 15 minutes so the caller's roles are read again on reconnect.
func (a *API) mcplEvents(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		auth.Problem(w, http.StatusInternalServerError, "Streaming isn't available here.")
		return
	}
	ch, cancel := a.events.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, ": following MCPL events\n\n")
	flusher.Flush()
	end := time.After(15 * time.Minute)
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-end:
			return
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e, open := <-ch:
			if !open {
				return
			}
			if !a.mayFollow(r, c, e) {
				continue
			}
			raw, _ := json.Marshal(e)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.ID, raw)
			flusher.Flush()
		}
	}
}

func (a *API) mayFollow(r *http.Request, c *caller, e mcpl.Event) bool {
	switch e.Access {
	case mcpl.SignedIn:
		return e.Actor == c.id.Sub
	case mcpl.Member:
		t := c.grantedTenant(e.Tenant)
		return t != nil && memberMay(c, t, e.Perm)
	case mcpl.Staff:
		return a.decideQuiet(r, c, e.Perm)
	}
	return false
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

// opMCPL lists every action for staff, marking the staff ones this caller may run.
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

// actionsFor is every action a signed-in caller could run here, for the manifest and MCP. Member
// actions are listed when the caller sees at least one organization.
func (a *API) actionsFor(r *http.Request, c *caller) []mcpl.Registered {
	var out []mcpl.Registered
	for _, g := range actions.All {
		switch g.Access {
		case mcpl.Staff:
			if !a.decideQuiet(r, c, g.Perm) {
				continue
			}
		case mcpl.Member:
			if !slices.ContainsFunc(c.tenants, func(t tenantView) bool { return memberMay(c, &t, g.Perm) }) {
				continue
			}
		}
		out = append(out, g)
	}
	return out
}
