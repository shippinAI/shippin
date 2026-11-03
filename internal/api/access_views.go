package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// opGrants lists every request and grant (newest first) plus the configured choices.
func (a *API) opGrants(w http.ResponseWriter, r *http.Request, _ *caller) {
	gs, err := a.db.AllGrants(r.Context(), 300)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Access grants are unavailable right now.")
		return
	}
	b := a.cfg.Access.BreakGlass
	writeJSON(w, map[string]any{"items": present(gs), "config": map[string]any{
		"durations": a.cfg.Access.Durations, "reason_min": a.cfg.Access.ReasonMin, "reason_max": a.cfg.Access.ReasonMax,
		"break_glass": map[string]any{"duration": b.Duration, "reauth_minutes": b.ReauthMinutes,
			"justification_min": b.JustificationMin, "categories": b.Categories},
	}})
}

// opAudit is the operator audit view: every entry (append-only), optionally for one tenant, plus a
// fresh walk of the whole hash chain (N2).
func (a *API) opAudit(w http.ResponseWriter, r *http.Request, _ *caller) {
	tid := r.URL.Query().Get("tenant_id")
	if tid != "" && !uuidRe.MatchString(tid) {
		auth.Problem(w, http.StatusBadRequest, "tenant_id must be a UUID.")
		return
	}
	es, err := a.db.AllAudit(r.Context(), tid, 500)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The audit log is unavailable right now.")
		return
	}
	chain, err := a.db.VerifyAuditChain(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The audit chain can't be verified right now.")
		return
	}
	writeJSON(w, map[string]any{"items": es, "chain": chain})
}
