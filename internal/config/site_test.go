package config

import (
	"encoding/json"
	"testing"
)

// B2-1: one-click Publish must stay off unless the config switches it on.
func TestSiteDefaultsOff(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"connect":{"enabled":false}}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Site.Enabled {
		t.Fatal("site.enabled must default to false")
	}
	if err := json.Unmarshal([]byte(`{"site":{"enabled":true}}`), &c); err != nil || !c.Site.Enabled {
		t.Fatal("site.enabled:true must switch it on")
	}
}

// idp.dialect is one short lowercase word; anything else fails closed.
func TestIDPDialectCheck(t *testing.T) {
	if got := (IDP{}).URN("org:project:roles"); got != "urn:nomen:iam:org:project:roles" {
		t.Fatalf("default spelling %q", got)
	}
	z := IDP{Dialect: "zitadel"}
	if z.APIAudience() != "urn:zitadel:iam:org:project:id:zitadel:aud" || z.OrgHeader() != "x-zitadel-orgid" {
		t.Fatalf("zitadel spelling %q %q", z.APIAudience(), z.OrgHeader())
	}
	for _, bad := range []string{"Zitadel", "urn:x", "a b", "x-y"} {
		if dialectRE.MatchString(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
