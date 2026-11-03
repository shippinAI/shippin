package api

import (
	"log"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// opSystem describes the control plane: its tenant, the hosts it owns, and whether the audit log is
// protected from the panel's own database role.
func (a *API) opSystem(w http.ResponseWriter, r *http.Request, _ *caller) {
	t, err := a.db.SystemTenant(r.Context())
	if err != nil {
		log.Printf("operator system: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The system tenant is unavailable right now.")
		return
	}
	hosts, err := a.hvDir().Endpoints(r.Context())
	if err != nil {
		log.Printf("operator system hosts: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The hosts are unavailable right now.")
		return
	}
	chain, err := a.db.VerifyAuditChain(r.Context())
	if err != nil {
		log.Printf("operator system audit: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The audit log can't be checked right now.")
		return
	}
	writeJSON(w, map[string]any{
		"tenant":     map[string]any{"id": t.ID, "name": t.Name, "status": t.Status, "created_at": t.CreatedAt},
		"hosts":      len(hosts),
		"audit":      chain,
		"self_guest": selfGuest(),
	})
}
