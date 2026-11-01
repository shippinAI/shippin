package main

import (
	"errors"
	"strings"
	"testing"
)

func TestLoginErrorsExplainRecovery(t *testing.T) {
	for _, tc := range []struct{ code, want string }{
		{"expired_token", "run login again and approve the new code"},
		{"access_denied", "no bridge access was granted"},
	} {
		if got := loginError(&apiError{400, tc.code}); !strings.Contains(got.Error(), tc.want) {
			t.Errorf("%s: %v", tc.code, got)
		}
	}
	err := errors.New("connection unavailable")
	if loginError(err) != err {
		t.Fatal("unexpected errors must retain the original diagnostic")
	}
}
