package api

import (
	"log"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// teamDeleteRole removes a role here and in Nomen (which drops it from every grant).
func (a *API) teamDeleteRole(w http.ResponseWriter, r *http.Request, c *caller) {
	key := r.PathValue("roleKey")
	old, err := a.roleByKey(r, key)
	if err != nil || old == nil {
		auth.Problem(w, http.StatusNotFound, "Role not found.")
		return
	}
	if n, err := a.ownersAfter(r.Context(), nil, nil, key); err != nil || n == 0 {
		auth.Problem(w, http.StatusConflict, "That would leave no active Owner.")
		return
	}
	if err := a.store().DeleteStaffRole(r.Context(), key, teamEntry(r, c, "team.role_delete", key+" \""+old.Name+"\"")); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The role couldn't be deleted.")
		return
	}
	if err := a.idp().DeleteRole(r.Context(), key); err != nil {
		log.Printf("team role delete %s: %v", key, err)
		_ = a.store().RestoreStaffRole(r.Context(), *old, teamEntry(r, c, "team.role_sync_failed", key+": "+a.cfg.IDP.Name+" refused; role restored"))
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" refused; the role is unchanged.")
		return
	}
	a.staffCache.flush()
	w.WriteHeader(http.StatusNoContent)
}

func toSet(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}
