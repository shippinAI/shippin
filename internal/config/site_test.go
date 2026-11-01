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
