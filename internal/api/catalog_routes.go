package api

import (
	"net/http"

	"shippin.cloud/console/internal/catalog"
)

// catalogRoutes adapts the customer catalog API (plans, services, customers, effective config,
// provision intents) to the gated operator routes; the staff caller is the actor. Each route is
// gated by its granular permission (deny by default); field-level checks (prices, contacts,
// notes) use the same decision, so the API enforces them, not only the UI.
func (a *API) catalogRoutes() []opRoute {
	// The store is bound per request so route listing works before a DB exists (gate tests).
	out := []opRoute{}
	for i, rt := range (&catalog.Handler{}).Routes() {
		out = append(out, opRoute{rt.Pattern, rt.Perm, func(w http.ResponseWriter, r *http.Request, c *caller) {
			h := &catalog.Handler{S: &catalog.Store{Pool: a.db.Pool}, Can: func(r *http.Request, perm string) bool {
				ok, _ := a.decide(r.Context(), c, perm)
				return ok
			}}
			h.Routes()[i].H(w, r, c.id.Sub)
		}})
	}
	return out
}
