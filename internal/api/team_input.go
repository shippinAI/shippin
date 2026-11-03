package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// checkRoles: every key must be an existing role; returns the roles table.
func (a *API) checkRoles(w http.ResponseWriter, r *http.Request, keys []string) ([]db.StaffRole, bool) {
	roles, err := a.store().StaffRoles(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Staff roles are unavailable right now.")
		return nil, false
	}
	for _, k := range keys {
		if !slices.ContainsFunc(roles, func(x db.StaffRole) bool { return x.Key == k }) {
			auth.Problem(w, http.StatusBadRequest, "Unknown role "+k+".")
			return nil, false
		}
	}
	return roles, true
}

func reasonOK(w http.ResponseWriter, s string) bool {
	if n := len(strings.TrimSpace(s)); n < 5 || n > 300 {
		auth.Problem(w, http.StatusBadRequest, "Give a reason (5 to 300 characters).")
		return false
	}
	return true
}

func uniq(xs []string) []string {
	out := []string{}
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func mustJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

// granularOK keeps the granular catalog permissions out of every role but the Owner preset:
// anyone else gets them one by one through an audited direct grant (deny by default).
func (in roleIn) granularOK(w http.ResponseWriter) bool {
	for _, p := range in.Permissions {
		if grantablePerm(p) && in.Key != "staff.owner" {
			auth.Problem(w, http.StatusBadRequest, p+" is granted per person (Team > Direct grants), not through a role.")
			return false
		}
	}
	return true
}
