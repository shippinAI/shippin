package effective_config_test

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

// The merge as the customer's effective configuration shows it: system locks hold, plan bounds clamp the
// customer's own values, and a workspace's layer applies only when asked for.
func TestEffectiveConfig(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.tenancy.customer.", mcpltest.As{Actor: "tester", Perms: func(string) bool { return true }})
	ctx := context.Background()
	if _, err := h.Pool.Exec(ctx, `INSERT INTO plans(id, label, rank, coverage) VALUES('scratch', 'Scratch', 5, 'foundational')`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Pool.Exec(ctx, `INSERT INTO plan_services(plan_id, service_id, enabled, lock, limits) VALUES('scratch', 'chatmaton', true, 'bounded', '{"seats":3}')`); err != nil {
		t.Fatal(err)
	}
	id := h.Call("POST", "/v1/operator/tenancy/customers", `{"name":"Acme","plan_id":"scratch","reason":"customer to configure"}`, 201)["id"].(string)
	h.Call("PUT", "/v1/operator/tenancy/customers/"+id+"/services/chatmaton", `{"overrides":{"seats":99},"reason":"more seats asked for"}`, 200)
	if _, err := h.Pool.Exec(ctx, `INSERT INTO tenant_defaults(layer, ref, key, value) VALUES('tenant', $1, 'workspace.idle_shutdown_min', '9999'),
		('workspace', 'ws1', 'workspace.idle_shutdown_min', '30')`, id); err != nil {
		t.Fatal(err)
	}
	path := "/v1/operator/tenancy/customers/" + id + "/effective-config"
	values := func(eff map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, s := range append(eff["settings"].([]any), eff["services"].([]any)...) {
			m := s.(map[string]any)
			out[m["key"].(string)] = m
		}
		return out
	}
	eff := h.Call("GET", path, "", 200)
	if eff["tenant_id"] != id || eff["plan_id"] != "scratch" || eff["workspace_id"] != nil || len(eff["layers"].([]any)) != 4 {
		t.Fatalf("effective config: %v", eff)
	}
	vals := values(eff)
	if v := vals["svc.chatmaton.seats"]; v["value"] != 3.0 || v["source"] != "tenant" {
		t.Fatalf("seats should clamp to the plan's max of 3: %v", v)
	}
	if v := vals["workspace.idle_shutdown_min"]; v["value"] != 480.0 {
		t.Fatalf("idle should clamp to the system max of 480: %v", v)
	}
	if v := vals["policy.sandbox"]; v["lock"] != "locked" || v["source"] != "system" {
		t.Fatalf("sandbox: %v", v)
	}
	if vals["svc.vaultix.enabled"]["value"] != true {
		t.Fatal("vaultix is required but scratch has no row: the plan's lock must not override the system's")
	}
	enabled := map[any]bool{}
	for _, s := range eff["enabled_services"].([]any) {
		enabled[s] = true
	}
	if !enabled["vaultix"] || !enabled["chatmaton"] {
		t.Fatalf("enabled services: %v", eff["enabled_services"])
	}

	ws := h.Call("GET", path+"?workspace=ws1", "", 200)
	if v := values(ws)["workspace.idle_shutdown_min"]; ws["workspace_id"] != "ws1" || v["value"] != 30.0 || v["source"] != "workspace" {
		t.Fatalf("workspace layer: %v %v", ws["workspace_id"], v)
	}
	h.Call("GET", "/v1/operator/tenancy/customers/00000000-0000-0000-0000-000000000000/effective-config", "", 404)
	h.Call("GET", "/v1/operator/tenancy/customers/nope/effective-config", "", 404)
	if len(h.Events) != 2 {
		t.Fatalf("reading emits no event: %+v", h.Events)
	}
}
