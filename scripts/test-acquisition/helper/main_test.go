package main

import (
	"os"
	"testing"
)

func TestFixtureOptInRequired(t *testing.T) {
	t.Setenv("SHIPPIN_ACQUISITION_DISPOSABLE", "")
	if run() == nil {
		t.Fatal("must reject without explicit disposable opt in")
	}
}
func TestFixtureRejectsUnknownPhase(t *testing.T) {
	t.Setenv("SHIPPIN_ACQUISITION_DISPOSABLE", "1")
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"helper", "not-supported"}
	if run() == nil {
		t.Fatal("must reject unsupported phase")
	}
}
func TestDeleteRejectsInvalidTargetBeforeReadingSecrets(t *testing.T) {
	for _, id := range []string{"", "../x", "https://real.example/lead", "abcdef", "0123456789abcdef0123456789abcdef/"} {
		t.Setenv("ACQUISITION_TEST_LEAD_ID", id)
		if err := deleteLead(); err == nil || err.Error() != "invalid lead id" {
			t.Fatal("invalid target must fail before secret or network access")
		}
	}
}
