package api

import (
	"context"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
)

// operatorURL is the operator app's own origin (ops.<domain>), or "" while it has none.
func (a *API) operatorURL() string {
	if a.cfg.Operator.Host == "" {
		return ""
	}
	return "https://" + a.cfg.Operator.Host
}

// operatorOrigin refuses the operator API on any host but the operator origin once
// operator.origin_only is set (CONSOLE-IA: separate origin and audience). The session cookie is
// host-only and tied to its app (auth.Flow.cookieSession), so a console sign-in never reaches it.
func (a *API) operatorOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Operator.OriginOnly && a.cfg.Operator.Host != "" && !a.flow.IsOperatorOrigin(r) {
			auth.Problem(w, http.StatusNotFound, "The provider panel API is served on "+a.operatorURL()+".")
			return
		}
		next(w, r)
	}
}

// idpAccountURL is the IdP's self-service page (idp.account_path under the issuer), "" if unset.
// The path is ops-editable (platform setting idp.account_path; the config file is the default).
func (a *API) idpAccountURL(ctx context.Context) string {
	p, _ := a.platformValue(ctx, "idp.account_path").(string)
	if p == "" || a.cfg.IDP.Issuer == "" {
		return ""
	}
	return strings.TrimRight(a.cfg.IDP.Issuer, "/") + p
}
