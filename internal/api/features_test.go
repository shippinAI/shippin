package api

import (
	"testing"

	"shippin.cloud/console/internal/config"
)

func TestFeaturesFollowTheSettings(t *testing.T) {
	a := &API{cfg: &config.Config{}}
	for k, on := range a.features() {
		if on {
			t.Errorf("%s is on with nothing configured", k)
		}
	}
	a.cfg.Incus.HostdURL = "http://127.0.0.1:8444"
	a.cfg.Catalog.Enabled = true
	a.cfg.Chat.Enabled = true
	a.cfg.Approval.Board.Driver = "hub"
	got := a.features()
	for _, k := range []string{"feature.workspaces", "feature.catalog", "feature.chat", "feature.board"} {
		if !got[k] {
			t.Errorf("%s must follow its setting", k)
		}
	}
	if got["feature.ai"] || got["feature.secrets"] {
		t.Errorf("only what was switched on: %v", got)
	}
}

func TestPlatformSettingsOfferOnlyWhatThisInstallationUses(t *testing.T) {
	keys := func(a *API) map[string]bool {
		out := map[string]bool{}
		for _, k := range a.platformKeys() {
			out[k.Key] = true
		}
		return out
	}
	a := &API{cfg: &config.Config{}}
	if got := keys(a); got["services.rrflow.auto_attach"] || got["idp.account_path"] || !got["services.probes"] {
		t.Fatalf("%v", got)
	}
	a.cfg.Services.RRFlow.Enabled = true
	a.cfg.IDP.ClientID = "client"
	if got := keys(a); !got["services.rrflow.auto_attach"] || !got["idp.account_path"] {
		t.Fatalf("%v", got)
	}
}
