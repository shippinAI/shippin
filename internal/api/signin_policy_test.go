package api

import "testing"

func TestAPlansSignInRuleIsMetOnlyByStrongEnoughSignIns(t *testing.T) {
	for _, c := range []struct {
		method, rule string
		want         bool
	}{
		{"password", "", true}, {"password", SignInPassword, true}, {"setup", SignInPassword, true},
		{"password", SignInSecondFactor, false}, {"recovery_code", SignInSecondFactor, false}, {"email_link", SignInSecondFactor, false},
		{"passkey", SignInSecondFactor, true}, {"identity_provider", SignInSecondFactor, true},
		{"passkey", SignInIdentityProvider, false}, {"identity_provider", SignInIdentityProvider, true},
		{"identity_provider", "something-new", false},
	} {
		if got := signInMeets(c.method, c.rule); got != c.want {
			t.Errorf("%s against %q = %v, want %v", c.method, c.rule, got, c.want)
		}
	}
}
