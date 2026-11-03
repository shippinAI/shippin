package api

import (
	"fmt"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

// teamSaveSettings replaces the team settings (audited with the write). Switching to enforce
// without the legacy fallback needs an Owner who has MFA, so the switch can't lock everyone out.
func (a *API) teamSaveSettings(w http.ResponseWriter, r *http.Request, c *caller) {
	var in struct {
		db.StaffSettings
		Reason string `json:"reason"`
	}
	if !decodeStrict(w, r, &in) || !reasonOK(w, in.Reason) {
		return
	}
	s := in.StaffSettings
	switch {
	case !config.ValidMode(s.Mode):
		auth.Problem(w, http.StatusBadRequest, "Mode must be legacy, shadow or enforce.")
		return
	case s.IdleDays < 1 || s.IdleDays > 3650:
		auth.Problem(w, http.StatusBadRequest, "Idle days must be between 1 and 3650.")
		return
	case s.ApprovalMinutes < 5 || s.ApprovalMinutes > 1440:
		auth.Problem(w, http.StatusBadRequest, "The approval window must be 5 to 1440 minutes.")
		return
	}
	if s.Mode == "enforce" && !s.LegacyFallback {
		roles, err := a.store().StaffRoles(r.Context())
		hs, herr := a.staffHolders(r.Context(), roles, nil)
		if err != nil || herr != nil || a.countWith(r.Context(), hs, permOwner, "", true) == 0 {
			auth.Problem(w, http.StatusConflict, "Enforcing without the IAM fallback needs at least one active Owner with MFA.")
			return
		}
	}
	old := a.teamSettings(r.Context())
	e := teamEntry(r, c, "team.settings", fmt.Sprintf("%+v → %+v · %s", old, s, in.Reason))
	if err := a.store().SaveStaffSettings(r.Context(), s, c.id.Sub, e); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The settings couldn't be saved.")
		return
	}
	a.staffCache.flush()
	writeJSON(w, s)
}
