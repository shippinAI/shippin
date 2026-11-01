package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// authzShadowStats (operators only): how often OpenFGA agreed with the legacy role logic since
// the console started. Shadow mode never changes a decision; this is the evidence for enforce.
func (a *API) authzShadowStats(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	if c.op == "" {
		auth.Problem(w, http.StatusForbidden, "Operators only.")
		return
	}
	if a.n2.fga == nil {
		writeJSON(w, map[string]string{"mode": "off"})
		return
	}
	writeJSON(w, a.n2.fga.Snapshot())
}
