package api

import (
	"strings"
	"testing"
)

func TestASiteWithSigningSettingsIsToldItsRecordsAreSafe(t *testing.T) {
	got := RetiredNotice(func(k string) string {
		return map[string]string{"GOLDSIGN_DIR": "/var/lib/sign", "GOLDSIGN_FACTS_DIR": "/var/lib/facts"}[k]
	})
	if !strings.Contains(got, "/var/lib/sign and /var/lib/facts are untouched") {
		t.Fatal(got)
	}
	if got := RetiredNotice(func(string) string { return "" }); got != "" {
		t.Fatalf("a new install should hear nothing: %q", got)
	}
}
