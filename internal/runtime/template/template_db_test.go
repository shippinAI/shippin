package template_test

import (
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/platform/config"
)

func rules() mcpl.Services {
	s := mcpl.Services{}
	mcpl.Provide[config.Workspaces](s, config.Workspaces{
		PortMin: 22000, PortMax: 22999, IDEPort: 3000, PreviewMin: 1024, PreviewMax: 65535, ImagePrefix: "shippin/",
		Sizes: []config.Size{{ID: "s", Label: "Small", CPU: 1, MemoryGiB: 2, DiskGiB: 10}},
		Guards: config.Guards{RepoHosts: []string{"github.com"},
			TemplateLimits: config.TemplateLimits{MaxCPU: 32, MaxMemoryGiB: 128, MinDiskGiB: 5, MaxDiskGiB: 1024, MaxPorts: 10, MaxStartup: 16384}},
	})
	return s
}

const base = "/v1/operator/runtime/templates"

func TestTemplateActions(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.runtime.", mcpltest.As{Actor: "staff", Services: rules()})
	before := len(h.Call("GET", base, "", 200)["items"].([]any))
	react := `"slug":"react","name":"React","image":"shippin/tpl-node","cpu":2,"memory_gib":4,"disk_gib":20,"ports":[5173],"startup":"npm run dev","enabled":true`
	h.Call("POST", base, `{`+react+`}`, 400) // no reason
	made := h.Call("POST", base, `{`+react+`,"reason":"offer React"}`, 201)
	id, _ := made["id"].(string)
	if id == "" || made["icon"] != "box" || made["startup"] != "npm run dev" {
		t.Fatalf("created: %v", made)
	}
	h.Call("POST", base, `{`+react+`,"reason":"same slug again"}`, 409)
	for _, bad := range []string{
		`"slug":"x1","name":"x","image":"docker.io/x","cpu":2,"memory_gib":4,"disk_gib":20`,                                           // an image from elsewhere
		`"slug":"x1","name":"x","image":"shippin/tpl-blank","cpu":2,"memory_gib":4,"disk_gib":20,"repo":"https://evil.example/a/b"`,   // a host that is not accepted
		`"slug":"x1","name":"x","image":"shippin/tpl-blank","cpu":2,"memory_gib":4,"disk_gib":20,"ports":[5173,5173]`,                 // a port twice
		`"slug":"x1","name":"x","image":"shippin/tpl-blank","cpu":99,"memory_gib":4,"disk_gib":20`,                                    // too large
		`"slug":"x1","name":"x","image":"shippin/tpl-blank","cpu":2,"memory_gib":4,"disk_gib":20,"created_at":"2020-01-01T00:00:00Z"`, // not a field staff write
	} {
		h.Call("POST", base, `{`+bad+`,"reason":"validation test"}`, 400)
	}
	got := h.Call("PUT", base+"/"+id, `{"slug":"react","name":"React 19","image":"shippin/tpl-node","cpu":4,"memory_gib":8,"disk_gib":20,"reason":"bigger default"}`, 200)
	if got["name"] != "React 19" || got["cpu"] != 4.0 || got["enabled"] != false {
		t.Fatalf("updated: %v", got)
	}
	h.Call("PUT", base+"/01a1424a-d04a-734f-abdb-9f8195327f66", `{"slug":"nope","name":"x","image":"shippin/tpl-node","cpu":2,"memory_gib":4,"disk_gib":20,"reason":"missing template"}`, 404)
	if n := len(h.Call("GET", base, "", 200)["items"].([]any)); n != before+1 {
		t.Fatalf("list has %d, want %d", n, before+1)
	}

	// A person choosing sees only what is turned on, without the start command.
	opts := h.Call("GET", "/v1/me/runtime/devspace-option", "", 200)
	for _, x := range opts["templates"].([]any) {
		if m := x.(map[string]any); m["id"] == id || m["startup"] != "" {
			t.Fatalf("options show a template that is off, or a start command: %v", m)
		}
	}
	h.Call("PUT", base+"/"+id, `{"slug":"react","name":"React 19","image":"shippin/tpl-node","cpu":4,"memory_gib":8,"disk_gib":20,"startup":"npm run dev","enabled":true,"reason":"turn it on"}`, 200)
	opts = h.Call("GET", "/v1/me/runtime/devspace-option", "", 200)
	found := false
	for _, x := range opts["templates"].([]any) {
		if m := x.(map[string]any); m["id"] == id {
			found = m["startup"] == ""
		}
	}
	rl, _ := opts["rules"].(map[string]any)
	if !found || len(opts["sizes"].([]any)) != 1 || rl["image_prefix"] != "shippin/" {
		t.Fatalf("options: %v", opts)
	}

	h.Call("DELETE", base+"/"+id, `{"reason":"no longer offered"}`, 204)
	h.Call("GET", base+"/"+id, "", 404)
	h.Call("DELETE", base+"/"+id, `{"reason":"again"}`, 404)
	if len(h.Audits) != 4 || len(h.Events) != 4 || h.Events[0].ID != "shippin.runtime.template.created" || h.Events[3].ID != "shippin.runtime.template.deleted" {
		t.Fatalf("audits %q events %+v", h.Audits, h.Events)
	}
}
