package api

import (
	"context"
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

// signInMeets says a session signed in by method satisfies a plan's rule. An unknown rule is treated as
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
		return "This organization's plan requires signing in through your identity provider. Sign out and sign in that way."
	}
	return "This organization's plan requires a second factor. Sign out and sign in with a passkey or your identity provider."
}

// planSignInRules maps each plan to its rule.
func (a *API) planSignInRules(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if a.db == nil {
		return out, nil
	}
	rows, err := a.db.Pool.Query(ctx, `SELECT id, sign_in FROM plans`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, rule string
		if err := rows.Scan(&id, &rule); err != nil {
			return nil, err
		}
		out[id] = rule
	}
	return out, rows.Err()
}
