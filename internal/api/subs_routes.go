package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/subs"
)

// subscriptionRoutes (M2): each user activates their OWN AI subscriptions by signing in with the
// vendor's CLI in their own terminal container. No route accepts or returns a credential.
func (a *API) subscriptionRoutes(mux Router) {
	mux.HandleFunc("GET /v1/me/subscriptions", a.flow.Require(a.mySubscriptions))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/subscriptions/{provider}/activate", a.flow.Require(a.activateSubscription))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/subscriptions/{provider}/check", a.flow.Require(a.checkSubscription))
	mux.HandleFunc("DELETE /v1/tenants/{tenantId}/subscriptions/{provider}", a.flow.Require(a.disconnectSubscription))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/ai-terminal/touch", a.flow.Require(a.touchTerminal))
	mux.HandleFunc("DELETE /v1/tenants/{tenantId}/ai-terminal", a.flow.Require(a.closeTerminal))
}

type subscriptionsOut struct {
	Enabled     bool                `json:"enabled"`
	TermsNote   string              `json:"terms_note"`
	TouchSecs   int                 `json:"touch_seconds"`
	Providers   []config.AIProvider `json:"providers"`
	Homes       []subs.Home         `json:"homes"`
	Connections []subs.Connection   `json:"connections"`
}

// mySubscriptions lists the caller's own AI homes and connection statuses, in tenants they
// still belong to as a member.
func (a *API) mySubscriptions(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	out := subscriptionsOut{Enabled: a.cfg.AI.Enabled, TermsNote: a.cfg.AI.TermsNote, TouchSecs: a.cfg.AI.TouchSeconds,
		Providers: a.cfg.AI.Providers, Homes: []subs.Home{}, Connections: []subs.Connection{}}
	if out.Providers == nil {
		out.Providers = []config.AIProvider{}
	}
	acct, err := a.subs.AccountBySub(r.Context(), c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return
	}
	if acct == "" {
		writeJSON(w, out)
		return
	}
	member := map[string]bool{}
	for i := range c.tenants {
		member[c.tenants[i].ID] = c.tenants[i].member()
	}
	homes, err1 := a.subs.Homes(r.Context(), acct)
	conns, err2 := a.subs.Connections(r.Context(), acct)
	if err1 != nil || err2 != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return
	}
	for _, h := range homes {
		if member[h.TenantID] {
			out.Homes = append(out.Homes, h)
		}
	}
	for _, cn := range conns {
		if member[cn.TenantID] {
			out.Connections = append(out.Connections, cn)
		}
	}
	writeJSON(w, out)
}

// subsCtx resolves a member caller in an AI-enabled tenant, their account and the provider.
func (a *API) subsCtx(w http.ResponseWriter, r *http.Request, wantProvider bool) (*caller, *tenantView, string, bool) {
	c, ok := a.resolve(w, r)
	if !ok {
		return nil, nil, "", false
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return nil, nil, "", false
	}
	if !t.member() || !a.cfg.AI.Allows(t.ID) {
		auth.Problem(w, http.StatusForbidden, "AI subscriptions aren't available in this tenant.")
		return nil, nil, "", false
	}
	if _, known := a.cfg.AI.Provider(r.PathValue("provider")); wantProvider && !known {
		auth.Problem(w, http.StatusNotFound, "Unknown AI provider.")
		return nil, nil, "", false
	}
	acct, err := a.db.UpsertAccount(r.Context(), c.id.Sub, c.id.Email)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return nil, nil, "", false
	}
	return c, t, acct, true
}

// terminalHost is the private activation terminal hostname of an account.
func (a *API) terminalHost(tenant, acct string) string {
	return subs.TerminalLabel(tenant, acct) + "." + a.cfg.Workspaces.BaseDomain
}
