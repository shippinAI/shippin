package api

import (
	"strings"
	"testing"

	"shippin.cloud/console/internal/config"
)

// Public settings expose only id and label; the IdP's internal ids stay server-side.
func TestLoginProvidersPublic(t *testing.T) {
	got := loginProviders([]config.IdPProvider{{ID: "google", Label: "Google", IdPID: "111"}, {ID: "github", IdPID: "222"}, {ID: "x", Label: "No id"}})
	if len(got) != 2 || got[0]["id"] != "google" || got[0]["label"] != "Google" || got[1]["label"] != "github" {
		t.Fatalf("%v", got)
	}
	for _, m := range got {
		for k, v := range m {
			if k != "id" && k != "label" || strings.Contains(v, "111") || strings.Contains(v, "222") {
				t.Fatalf("leaked %s=%s", k, v)
			}
		}
	}
	if loginProviders(nil) == nil {
		t.Fatal("nil providers must encode as []")
	}
}
