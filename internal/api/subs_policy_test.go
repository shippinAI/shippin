package api

import (
	"testing"

	"shippin.cloud/console/internal/subs"
)

func TestSubscriptionPolicy(t *testing.T) {
	a := &API{cfg: testCfg()}
	for host, ok := range map[string]bool{"u-0123456789ab.ws.example.test": true, "u-abcdef-123456.ws.example.test": false,
		"u-0123456789ab.evil.test": false, "u-0123456789AB.ws.example.test": false} {
		if a.utilityHost(host) != ok {
			t.Errorf("%s: %v", host, !ok)
		}
	}
	for _, c := range [][3]string{{"", "t", subs.Connected}, {subs.Connected, "", subs.NeedsReauth},
		{subs.Activating, "", subs.Activating}, {"", "", subs.NotConnected}, {subs.Disconnected, "", subs.Disconnected}} {
		if got := nextStatus(c[0], c[1] != ""); got != c[2] {
			t.Errorf("nextStatus(%q,%v)=%s", c[0], c[1] != "", got)
		}
	}
	if subs.UtilityLabel("019a0000-0000-7000-8000-0123456789ab") != "u-0123456789ab" {
		t.Error("terminal label uses the random tail of the account id")
	}
}
