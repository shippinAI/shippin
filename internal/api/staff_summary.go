package api

import "net/http"

// staffSummary is the caller's staff state for /me and the panel shell. permissions is the
// effective set under the current mode, from the same decide() the API gates use.
func (a *API) staffSummary(r *http.Request, c *caller) map[string]any {
	mode := a.authzMode(r.Context())
	eff := []string{}
	legacy := false
	for _, p := range permCatalog {
		if ok, note := a.decide(r.Context(), c, p.Key); ok {
			eff = append(eff, p.Key)
			legacy = legacy || note == "legacy"
		}
	}
	out := map[string]any{"mode": mode, "permissions": eff, "legacy_fallback": legacy || (mode != "enforce" && a.isPlatformAdmin(c)),
		"roles": []string{}, "mfa": false, "mfa_required": false}
	if mode != "legacy" {
		v := a.staffOf(r.Context(), c)
		out["roles"], out["mfa"], out["mfa_required"], out["unavailable"] = v.Roles, v.MFA, v.MFARequired, v.Err
	}
	return out
}

// panelAllowed is the "Admin panel" switch: any effective permission.
func (a *API) panelAllowed(r *http.Request, c *caller) bool {
	ok, _ := a.decide(r.Context(), c, permPanel)
	return ok
}

func (a *API) decideQuiet(r *http.Request, c *caller, perm string) bool {
	ok, _ := a.decide(r.Context(), c, perm)
	return ok
}
