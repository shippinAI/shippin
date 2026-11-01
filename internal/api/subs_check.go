package api

import (
	"context"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/subs"
)

// checkTimeout bounds one status read through hostd (it runs the vendor status probes).
const checkTimeout = 45 * time.Second

// checkSubscription reads the isolated owner's runner and updates every configured provider.
// Shared workspaces are never sources of provider authentication status.
func (a *API) checkSubscription(w http.ResponseWriter, r *http.Request) {
	c, t, acct, ok := a.subsCtx(w, r, true)
	if !ok {
		return
	}
	h, err := a.subs.HomeFor(r.Context(), t.ID, acct, "")
	if err == subs.ErrNotFound {
		auth.Problem(w, http.StatusConflict, "Activate a subscription first.")
		return
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	st, err := a.host.UtilityAIStatus(ctx, h.TenantID, h.AccountID)
	if err != nil || !validAIReport(st) {
		if err := a.invalidateStatus(r.Context(), h); err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "Sign-in verification failed and could not be recorded. Subscription work is unavailable.")
			return
		}
		auth.Problem(w, http.StatusConflict, "Sign-in could not be verified. Open your private AI terminal and check again.")
		return
	}
	if err := a.applyFreshStatus(r.Context(), h, st); err != nil {
		_ = a.invalidateStatus(r.Context(), h)
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return
	}
	a.resumeWaiting(ctx, h) // M4: builds parked on this sign-in start now
	conns, err := a.subs.Connections(r.Context(), acct)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return
	}
	visible := []subs.Connection{}
	for _, cn := range conns {
		for _, tn := range c.tenants {
			if tn.ID == cn.TenantID && tn.member() {
				visible = append(visible, cn)
				break
			}
		}
	}
	writeJSON(w, map[string]any{"connections": visible})
}

// Host timeouts and browser disconnects must not cancel the deny-state write.
func (a *API) invalidateStatus(parent context.Context, h *subs.Home) error {
	return a.applyFreshStatus(parent, h, &hostclient.AIStatus{})
}

func (a *API) applyFreshStatus(parent context.Context, h *subs.Home, st *hostclient.AIStatus) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	return a.applyStatus(ctx, h, st)
}

func validAIReport(st *hostclient.AIStatus) bool {
	if st == nil || st.Version != 2 || time.Since(st.CheckedAt) > checkTimeout+5*time.Second || time.Until(st.CheckedAt) > 5*time.Second {
		return false
	}
	seen := map[string]bool{}
	for _, p := range st.Providers {
		if seen[p.Provider] || p.Provider == "" || (p.SignedIn && (!p.Installed || !p.AuthKnown)) {
			return false
		}
		seen[p.Provider] = true
	}
	return true
}

// applyStatus maps "signed in or not" onto each configured provider's connection row.
func (a *API) applyStatus(ctx context.Context, h *subs.Home, st *hostclient.AIStatus) error {
	prev := map[string]string{}
	if conns, err := a.subs.Connections(ctx, h.AccountID); err == nil {
		for _, c := range conns {
			if c.HomeID == h.ID {
				prev[c.ProviderID] = c.Status
			}
		}
	}
	statuses := map[string]string{}
	for _, configured := range a.cfg.AI.Providers {
		status := subs.Limited // Missing/unsupported probes never retain a previous connected state.
		for _, p := range st.Providers {
			if p.Provider == configured.ID && p.Installed && p.AuthKnown {
				status = nextStatus(prev[p.Provider], p.SignedIn)
				break
			}
		}
		statuses[configured.ID] = status
	}
	return a.subs.SetReport(ctx, h, statuses)
}

// nextStatus: signed in → connected; a lost sign-in after connected → needs_reauth; otherwise
// keep activating/disconnected as they were, or not_connected.
func nextStatus(prev string, signedIn bool) string {
	switch {
	case signedIn:
		return subs.Connected
	case prev == subs.Connected || prev == subs.NeedsReauth:
		return subs.NeedsReauth
	case prev == subs.Activating || prev == subs.Disconnected:
		return prev
	}
	return subs.NotConnected
}
