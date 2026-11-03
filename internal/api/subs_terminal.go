package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/subs"
)

// disconnectSubscription runs the vendor's own sign-out in the caller's terminal container (the
// login is removed from their AI home), starting the terminal first when it is stopped.
func (a *API) disconnectSubscription(w http.ResponseWriter, r *http.Request) {
	c, t, acct, ok := a.subsCtx(w, r, true)
	if !ok {
		return
	}
	h, err := a.subs.HomeFor(r.Context(), t.ID, acct, "")
	if err == subs.ErrNotFound {
		writeJSON(w, map[string]string{"status": subs.NotConnected})
		return
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return
	}
	p := r.PathValue("provider")
	// Revoke use before vendor logout or job enqueue. Reports cannot undo this;
	// only a new explicit activation followed by verification can reconnect.
	if err := a.subs.SetStatus(r.Context(), h, p, subs.Disconnected, "", true); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-out could not be recorded. Try again.")
		return
	}
	if h.UtilityState == "running" {
		if err := a.host.UtilityLogout(r.Context(), t.ID, acct, p); err == nil {
			_ = a.subs.SetStatus(r.Context(), h, p, subs.Disconnected, "", true)
			writeJSON(w, map[string]string{"status": subs.Disconnected})
			return
		}
	}
	_ = a.subs.SetUtility(r.Context(), h.ID, "starting", true)
	j, err := a.startAIJob(r.Context(), w, c, h, aiJob{Logout: p})
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sign-out could not be queued. Try again.")
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]any{"status": "disconnecting", "job": j})
}

// touchTerminal keeps the caller's open terminal from the idle stop.
func (a *API) touchTerminal(w http.ResponseWriter, r *http.Request) {
	_, t, acct, ok := a.subsCtx(w, r, false)
	if !ok {
		return
	}
	h, err := a.subs.HomeFor(r.Context(), t.ID, acct, "")
	if err != nil {
		auth.Problem(w, http.StatusNotFound, "No AI terminal.")
		return
	}
	if err := a.subs.TouchUtility(r.Context(), h.ID); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The AI terminal could not be kept active.")
		return
	}
	writeJSON(w, map[string]string{"utility_state": h.UtilityState})
}

// closeTerminal stops the caller's terminal now (their logins stay on the AI home).
func (a *API) closeTerminal(w http.ResponseWriter, r *http.Request) {
	_, t, acct, ok := a.subsCtx(w, r, false)
	if !ok {
		return
	}
	h, err := a.subs.HomeFor(r.Context(), t.ID, acct, "")
	if errors.Is(err, subs.ErrNotFound) {
		writeJSON(w, map[string]string{"utility_state": "stopped"})
		return
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The AI terminal state could not be checked.")
		return
	}
	if err := a.host.StopUtility(r.Context(), t.ID, acct); err != nil {
		log.Printf("AI terminal %s stop/cleanup: %v", h.ID, errors.Join(err, a.utilityFailed(r.Context(), h.ID)))
		auth.Problem(w, http.StatusBadGateway, "The AI terminal could not be stopped. Try again.")
		return
	}
	if err := a.utilityStopped(r.Context(), h.ID); err != nil {
		log.Printf("AI terminal %s stopped but state write failed: %v", h.ID, err)
		auth.Problem(w, http.StatusServiceUnavailable, "The terminal stopped, but its status could not be saved.")
		return
	}
	writeJSON(w, map[string]string{"utility_state": "stopped"})
}

// utilityHost: an activation terminal hostname (u-<12 hex>.<ws base domain>).
func (a *API) utilityHost(host string) bool {
	label, ok := strings.CutSuffix(host, "."+a.cfg.Workspaces.BaseDomain)
	if !ok || (len(label) != 14 && len(label) != 34) || !strings.HasPrefix(label, "u-") {
		return false
	}
	return strings.Trim(label[2:], "0123456789abcdef") == "" // workspace labels always carry a dash
}

// utilityAllowed: only the owner of the AI home may open its terminal. No manager, operator or
// grant holder can, because the terminal holds that user's own vendor sign-ins.
func (a *API) utilityAllowed(r *http.Request, tok, host string) bool {
	c, err := a.callerFor(r, tok)
	if err == nil && c.founder {
		return a.founderOpen(r, c, host)
	}
	return err == nil && a.utilityOwner(r.Context(), c, host)
}

func (a *API) utilityOwner(ctx context.Context, c *caller, host string) bool {
	h, err := a.subs.HomeByHost(ctx, host)
	if err != nil || c == nil || c.id == nil {
		return false
	}
	if h.UtilityHost != a.terminalHost(h.TenantID, h.AccountID) {
		return false // Legacy account-only hosts need migration before they can be reopened.
	}
	acct, err := a.subs.AccountBySub(ctx, c.id.Sub)
	if err != nil || acct == "" || acct != h.AccountID {
		return false
	}
	t := c.grantedTenant(h.TenantID)
	return t != nil && t.member()
}

// stopIdleTerminals stops terminals nobody has had open for ai.idle_minutes (reconciler).
func (a *API) stopIdleTerminals(ctx context.Context) {
	if !a.cfg.AI.Enabled {
		return
	}
	idle, err := a.subs.IdleUtilities(ctx, config.Sec(a.cfg.AI.IdleMinutes*60))
	if err != nil {
		log.Printf("reconcile: idle AI terminals: %v", err)
		return
	}
	for _, h := range idle {
		if err := a.host.StopUtility(ctx, h.TenantID, h.AccountID); err != nil {
			log.Printf("reconcile: stop AI terminal %s: %v", h.ID, errors.Join(err, a.utilityFailed(ctx, h.ID)))
			continue
		}
		if err := a.utilityStopped(ctx, h.ID); err != nil {
			log.Printf("reconcile: AI terminal %s stopped but state write failed: %v", h.ID, err)
		}
	}
}
