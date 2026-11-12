package api

import (
	"net/http"

	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/mcpl"
)

// MCPL (docs/mcpl.md): every action described once, for people, the API and an AI acting for a person.
// A context's actions sit next to its routes; until a context is rewritten into its own package, its
// described actions are listed here. TestMCPLConformance checks them against the served routes.

func (a *API) actions() []mcpl.Action {
	out := []mcpl.Action{
		{ID: "shippin.public.mcpl.get", Route: "GET /v1/mcpl", Access: mcpl.Public, Risk: mcpl.Read, Summary: "Describe this installation's MCPL domains and public actions."},
		{ID: "shippin.public.settings.get", Route: "GET /v1/settings/public", Access: mcpl.Public, Risk: mcpl.Read, Summary: "Read the brand, sign-in options and sign-up mode shown before sign-in."},
		{ID: "shippin.public.waitlist.join", Route: "POST /v1/waitlist", Access: mcpl.Public, Risk: mcpl.Write, Summary: "Ask to join the waitlist, when it is open."},
		{ID: "shippin.public.waitlist.confirm", Route: "POST /v1/waitlist/confirm", Access: mcpl.Public, Risk: mcpl.Write, Summary: "Confirm a waitlist address with the emailed code."},
		{ID: "shippin.auth.setup.redeem", Route: "POST /v1/setup", Access: mcpl.Public, Risk: mcpl.Write, Summary: "Spend the one-time setup code on the built-in setup account, organization and brand."},
		{ID: "shippin.auth.setup_handover.get", Route: "GET /v1/setup/handover", Access: mcpl.SignedIn, Risk: mcpl.Read, Summary: "Read how far the owner is in taking over the setup account."},
		{ID: "shippin.auth.setup_account.take_over", Route: "POST /v1/setup/handover/password", Access: mcpl.SignedIn, Risk: mcpl.Write, Summary: "Set the setup account's password and sign out every other session of it."},
		{ID: "shippin.identity.owner_account.create", Route: "POST /v1/setup/handover/owner", Access: mcpl.SignedIn, Risk: mcpl.Write, Summary: "Create the owner's own account, returning its recovery codes once."},
		{ID: "shippin.platform.mcpl.list", Route: "GET /v1/operator/mcpl", Access: mcpl.Staff, Perm: permPanel, Risk: mcpl.Read, Summary: "List every described action and whether the caller may run it."},
		{ID: "shippin.platform.setting.list", Route: "GET /v1/operator/platform-settings", Access: mcpl.Staff, Perm: "platform.settings.read", Risk: mcpl.Read, Summary: "List the settings that can be changed in the panel, with their values."},
		{ID: "shippin.platform.setting.set", Route: "PUT /v1/operator/platform-settings/{key}", Access: mcpl.Staff, Perm: "platform.settings.write", Risk: mcpl.Write, Reason: true, Summary: "Change a setting, or return it to the config file's value."},
		{ID: "shippin.identity.waitlist.list", Route: "GET /v1/operator/waitlist", Access: mcpl.Staff, Perm: "customers.read", Risk: mcpl.Read, Summary: "List the people on the waitlist and the sign-up mode."},
		{ID: "shippin.identity.waitlist.invite", Route: "POST /v1/operator/waitlist/{id}/invite", Access: mcpl.Staff, Perm: "customers.write", Risk: mcpl.Write, Reason: true, Summary: "Invite a confirmed person off the waitlist."},
	}
	for _, rt := range (&catalog.Handler{}).Routes() {
		out = append(out, rt.Action())
	}
	return out
}

// mcplPublic answers anyone: the contract version, the domains and the actions that need no session.
func (a *API) mcplPublic(w http.ResponseWriter, _ *http.Request) {
	public := []mcpl.Action{}
	for _, x := range a.actions() {
		if x.Access == mcpl.Public {
			public = append(public, x)
		}
	}
	writeJSON(w, map[string]any{"mcpl": mcpl.Version, "domains": mcpl.Domains, "actions": public})
}

// opMCPL lists every described action for staff, marking the ones this caller may run.
func (a *API) opMCPL(w http.ResponseWriter, r *http.Request, c *caller) {
	type row struct {
		mcpl.Action
		Allowed bool `json:"allowed"`
	}
	rows := []row{}
	for _, x := range a.actions() {
		allowed := x.Access != mcpl.Staff || x.Perm == permPanel
		if x.Access == mcpl.Staff && x.Perm != permPanel {
			allowed, _ = a.decide(r.Context(), c, x.Perm)
		}
		rows = append(rows, row{x, allowed})
	}
	writeJSON(w, map[string]any{"mcpl": mcpl.Version, "domains": mcpl.Domains, "actions": rows})
}

func (a *API) mcplRoutes() []opRoute {
	return []opRoute{{"GET /v1/operator/mcpl", permPanel, a.opMCPL}}
}
