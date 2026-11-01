package hostclient

import (
	"errors"
	"fmt"
	"testing"
)

func TestRejected(t *testing.T) {
	for code, want := range map[int]bool{400: true, 403: true, 404: true, 408: false, 409: false, 429: false, 500: false, 503: false} {
		err := fmt.Errorf("wrapped: %w", &StatusError{Code: code, Msg: "x"})
		if got := Rejected(err); got != want {
			t.Errorf("Rejected(%d)=%v want %v", code, got, want)
		}
	}
	if Rejected(errors.New("connection refused")) {
		t.Error("a transport error is retryable")
	}
}
