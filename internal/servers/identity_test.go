package servers

import (
	"strings"
	"testing"
)

func TestSystemIdentityValidation(t *testing.T) {
	valid := SystemIdentity{ServerID: "019a0000-0000-7000-8000-000000000001", Label: "Control plane", Aliases: []string{"rack-a", "control-plane"}, Tags: []string{"prod"}}
	if !validIdentityValues(valid) {
		t.Fatal("valid descriptive identity rejected")
	}
	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = "alias-" + string(rune('a'+i))
	}
	for name, v := range map[string]SystemIdentity{
		"mapping half supplied": {AgentHubTenantID: "tenant"},
		"duplicate aliases":     {Aliases: []string{"edge", "EDGE"}},
		"control character":     {Tags: []string{"prod\nsecond"}},
		"too many aliases":      {Aliases: tooMany},
		"too long ID":           {AgentHubTenantID: strings.Repeat("t", 256), AgentHubMachineID: "machine"},
	} {
		v.Label = "Control plane"
		if validIdentityValues(v) {
			t.Errorf("%s should be rejected", name)
		}
	}
}
