package api

import (
	"context"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/tenancy/coverage_tier"
)

// Sign-in rules a plan can set for the members of its organizations, weakest first.
const (
	SignInPassword         = "password"
	SignInSecondFactor     = "second_factor"
	SignInIdentityProvider = "identity_provider"
)

// signInStrength ranks how a session was signed in. A passkey is a second factor; an identity provider
// applies its own policy and counts as the strongest.
func signInStrength(method string) int {
	switch method {
	case "identity_provider":
		return 2
	case "passkey":
		return 1
	}
	return 0
}

func signInRequired(rule string) int {
	switch rule {
	case SignInIdentityProvider:
		return 2
	case SignInSecondFactor:
		return 1
	}
	return 0
}

// signInMeets says a session signed in by method satisfies a coverage tier's rule. An unknown rule is treated as
// the strictest, so a mistake never opens access.
func signInMeets(method, rule string) bool {
	switch rule {
	case "", SignInPassword, SignInSecondFactor, SignInIdentityProvider:
		return signInStrength(method) >= signInRequired(rule)
	}
	return false
}

func signInRefusal(rule string) string {
	if rule == SignInIdentityProvider {
		return "This organization's coverage requires signing in through your identity provider. Sign out and sign in that way."
	}
	return "This organization's coverage requires a second factor. Sign out and sign in with a passkey or your identity provider."
}

// tenantCoverage maps each tenant to its effective coverage tier and that tier's sign-in rule.
func (a *API) tenantCoverage(ctx context.Context, ids []string) (map[string]coverage_tier.Coverage, error) {
	if a.db == nil || len(ids) == 0 {
		return map[string]coverage_tier.Coverage{}, nil
	}
	return coverage_tier.Of(ctx, a.db.Pool, ids)
}

// coverage is GET /v1/tenants/{tenantId}/coverage: what the organization's coverage tier includes,
// for every member. Prices stay with the operator.
func (a *API) coverage(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if a.db == nil {
		writeJSON(w, map[string]any{"coverage": nil})
		return
	}
	view, err := coverage_tier.ForTenant(r.Context(), a.db.Pool, t.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Coverage is unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"coverage": view})
}
