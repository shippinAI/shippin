package catalog_service_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const base = "/v1/operator/services/catalog-services"

func serve(t *testing.T) *mcpltest.Harness {
	return mcpltest.Serve(t, actions.All, "shippin.services.catalog_service.", mcpltest.As{Actor: "tester", Perms: func(string) bool { return true }})
}

// with adds a reason to a request body.
func with(body, reason string) string {
	return strings.TrimSuffix(body, "}") + `,"reason":"` + reason + `"}`
}

// Ported from the legacy TestCatalogAPIFlow: definitions, pinned sources and runtime contracts, with the
// same refusals.
func TestCatalogServiceDefinitions(t *testing.T) {
	h := serve(t)
	seed := h.Call("GET", base, "", 200)["items"].([]any)
	if len(seed) == 0 || seed[0].(map[string]any)["required"] != true {
		t.Fatalf("seed services, required first: %v", seed)
	}
	h.Call("POST", base, `{"id":"backup","label":"Backups","default_config":{"days":7}}`, 400) // no reason
	backup := h.Call("POST", base, with(`{"id":"backup","label":"Backups","default_config":{"days":7}}`, "sell backups"), 201)
	if backup["kind"] != "app" || backup["active"] != true || backup["required"] != false || backup["deployment_source"] != nil ||
		backup["default_config"].(map[string]any)["days"] != 7.0 || len(backup["config_schema"].(map[string]any)) != 0 || backup["updated_at"] == "" {
		t.Fatalf("defaults not applied: %v", backup)
	}
	if got := h.Call("GET", base+"/backup", "", 200); got["label"] != "Backups" {
		t.Fatalf("get: %v", got)
	}
	h.Call("GET", base+"/nope", "", 404)
	h.Call("POST", base, with(`{"id":"backup","label":"Again"}`, "duplicate id"), 409)
	h.Call("POST", base, with(`{"id":"Bad Id","label":"x"}`, "bad id"), 400)
	h.Call("POST", base, with(`{"id":"nolabel"}`, "no label"), 400)
	h.Call("POST", base, with(`{"id":"arr","label":"x","default_config":[1]}`, "defaults not an object"), 400)
	h.Call("POST", base, with(`{"id":"nul","label":"x","config_schema":null}`, "schema null"), 400)
	h.Call("POST", base, with(`{"id":"extra","label":"x","colour":"blue"}`, "unknown field"), 400)
	h.Call("PUT", base+"/backup", with(`{"id":"backup","label":"Backups"}`, "id belongs in the path"), 400)

	source := `"deployment_source":{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","credential_ref":"vaultix://production/runtime/registry-token"}`
	runtime := `"runtime_contract":{"version":1,"kind":"incus_oci_container","providers":["incus"]}`
	configSchema := `"config_schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"port":{"type":"integer","minimum":1,"maximum":65535},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false},"default_config":{"port":5432,"password":"vaultix://production/postgres/password"}`
	post := func(body, reason string, want int) map[string]any {
		return h.Call("POST", base, with(body, reason), want)
	}
	post(`{"id":"unsafe-schema","label":"Unsafe","kind":"database",`+source+`,"config_schema":{"type":"object"}}`, "schema without policy", 400)
	metadataOnly := post(`{"id":"metadata-only","label":"Metadata only","kind":"database",`+configSchema+`,`+source+`}`, "source without runtime", 201)
	if metadataOnly["runtime_contract"] != nil {
		t.Fatalf("metadata-only service unexpectedly declares a runtime: %v", metadataOnly["runtime_contract"])
	}
	post(`{"id":"orphan-runtime","label":"Orphan","kind":"database",`+runtime+`}`, "runtime without source", 400)
	post(`{"id":"unsupported-runtime","label":"Unsafe","kind":"database",`+configSchema+`,`+source+`,"runtime_contract":{"version":1,"kind":"proxmox_cloud_init","providers":["proxmox"]}}`, "unsupported runtime", 400)
	post(`{"id":"plaintext-secret","label":"Unsafe","kind":"database",`+source+`,`+configSchema[:strings.Index(configSchema, `,"default_config"`)]+`,"default_config":{"port":5432,"password":"plaintext-password"}}`, "plaintext secret", 400)
	configured := post(`{"id":"postgres","label":"Postgres","kind":"database",`+configSchema+`,`+source+`,`+runtime+`}`, "deployable postgres", 201)
	if configured["deployment_source"].(map[string]any)["kind"] != "oci" || configured["runtime_contract"].(map[string]any)["kind"] != "incus_oci_container" {
		t.Fatalf("source or runtime contract not returned after create: %v", configured)
	}
	recipe := `"runtime_contract":{"version":2,"kind":"incus_oci_container","providers":["incus"],"resources":{"cpu_cores":2,"memory_mib":2048},"storage":{"size_gib":20,"mount_path":"/var/lib/postgresql/data"},"network":{"exposure":"private","ports":[{"name":"postgres","protocol":"tcp","container_port":5432}]},"health":{"kind":"tcp","port":5432,"interval_seconds":30,"timeout_seconds":5,"failure_threshold":3}}`
	v2 := post(`{"id":"runtime-recipe-v2","label":"Runtime recipe v2","kind":"database",`+configSchema+`,`+source+`,`+recipe+`}`, "v2 recipe", 201)
	if got := v2["runtime_contract"].(map[string]any)["version"]; got != 2.0 {
		t.Fatalf("version 2 runtime recipe not returned: %v", v2["runtime_contract"])
	}
	recipeV3 := strings.Replace(recipe, `"version":2`, `"version":3`, 1)
	recipeV3 = strings.TrimSuffix(recipeV3, "}") + `,"environment":[{"name":"POSTGRES_PASSWORD","config_path":"/password"}]}`
	v3 := post(`{"id":"runtime-recipe-v3","label":"Runtime recipe v3","kind":"database",`+configSchema+`,`+source+`,`+recipeV3+`}`, "v3 recipe", 201)
	if c := v3["runtime_contract"].(map[string]any); c["version"] != 3.0 || len(c["environment"].([]any)) != 1 {
		t.Fatalf("version 3 runtime binding did not persist: %v", c)
	}
	badBinding := strings.Replace(recipeV3, `"config_path":"/password"`, `"config_path":"/missing"`, 1)
	post(`{"id":"bad-binding","label":"Bad binding","kind":"database",`+configSchema+`,`+source+`,`+badBinding+`}`, "binding to nothing", 400)
	post(`{"id":"bad-recipe","label":"Bad recipe","kind":"database",`+configSchema+`,`+source+`,"runtime_contract":{"version":2,"kind":"incus_oci_container","providers":["incus"],"resources":{"cpu_cores":0,"memory_mib":2048}}}`, "incomplete recipe", 400)

	loaded := h.Call("GET", base+"/postgres", "", 200)
	if loaded["deployment_source"].(map[string]any)["credential_ref"] != "vaultix://production/runtime/registry-token" {
		t.Fatalf("source not persisted: %v", loaded["deployment_source"])
	}
	h.Call("PUT", base+"/postgres", with(`{"label":"Postgres","deployment_source":{"kind":"github","reference":"https://github.com/acme/postgres","commit":"main"}}`, "floating commit"), 400)
	h.Call("PUT", base+"/nope", with(`{"label":"Nope"}`, "missing service"), 404)
	h.Call("PUT", base+"/postgres", with(`{"label":"Postgres","kind":"database",`+configSchema+`,"deployment_source":{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},`+runtime+`}`, "drop the registry token"), 200)
	loaded = h.Call("GET", base+"/postgres", "", 200)
	if _, ok := loaded["deployment_source"].(map[string]any)["credential_ref"]; ok {
		t.Fatalf("update did not replace the source contract: %v", loaded["deployment_source"])
	}
	updated := h.Call("PUT", base+"/backup", with(`{"label":"Backups+","active":false,"required":true}`, "make backups required"), 200)
	if updated["label"] != "Backups+" || updated["active"] != false || updated["required"] != true || len(updated["default_config"].(map[string]any)) != 0 {
		t.Fatalf("update replaces every field: %v", updated)
	}

	// Only changes that happened are audited, each with its reason; refused attempts are not.
	want := []string{
		"shippin.services.catalog_service.create id=backup · sell backups",
		"shippin.services.catalog_service.create id=metadata-only · source without runtime",
		"shippin.services.catalog_service.create id=postgres · deployable postgres",
		"shippin.services.catalog_service.create id=runtime-recipe-v2 · v2 recipe",
		"shippin.services.catalog_service.create id=runtime-recipe-v3 · v3 recipe",
		"shippin.services.catalog_service.update catalogServiceId=postgres · drop the registry token",
		"shippin.services.catalog_service.update catalogServiceId=backup · make backups required",
	}
	if strings.Join(h.Audits, "\n") != strings.Join(want, "\n") {
		t.Fatalf("audits:\n%s", strings.Join(h.Audits, "\n"))
	}
	if len(h.Events) != len(want) || h.Events[0].ID != "shippin.services.catalog_service.created" || h.Events[0].Subject["id"] != "backup" ||
		h.Events[6].ID != "shippin.services.catalog_service.updated" || h.Events[6].Subject["catalogServiceId"] != "backup" ||
		h.Events[6].Access != "staff" || h.Events[6].Perm != "catalog.plans.write" {
		t.Fatalf("events: %+v", h.Events)
	}
}

type stmt struct {
	sql  string
	args []any
}

func exec(t *testing.T, h *mcpltest.Harness, stmts ...stmt) {
	t.Helper()
	for _, s := range stmts {
		if _, err := h.Pool.Exec(t.Context(), s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
}

// Deleting a service takes it out of the plans that include it and organizations' settings for it, as the
// legacy route did; a deployment that uses it keeps it in the catalog.
func TestCatalogServiceDelete(t *testing.T) {
	h := serve(t)
	h.Call("POST", base, with(`{"id":"backup","label":"Backups"}`, "sell backups"), 201)
	tenant, server := uuid.NewString(), uuid.NewString()
	exec(t, h,
		stmt{`INSERT INTO plan_services(plan_id,service_id) VALUES('starter','backup')`, nil},
		stmt{`INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES($1,$2,'Acme','starter','active')`, []any{tenant, uuid.NewString()}},
		stmt{`INSERT INTO tenant_services(tenant_id,service_id,enabled) VALUES($1,'backup',true)`, []any{tenant}})
	h.Call("DELETE", base+"/backup", ``, 400) // no reason
	h.Call("DELETE", base+"/nope", `{"reason":"missing service"}`, 404)
	h.Call("DELETE", base+"/backup", `{"reason":"no longer sold"}`, 204)
	h.Call("GET", base+"/backup", "", 404)
	var left int
	if err := h.Pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM plan_services WHERE service_id='backup') +
		(SELECT count(*) FROM tenant_services WHERE service_id='backup')`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("plan and organization rows left behind: %d %v", left, err)
	}

	exec(t, h,
		stmt{`INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind) VALUES($1,$2,'target','Target','test','active','test','incus')`, []any{server, tenant}},
		stmt{`INSERT INTO server_assignments(server_id,tenant_id,role,created_by) VALUES($1,$2,'owner','test')`, []any{server, tenant}},
		stmt{`INSERT INTO shippin_deployments(id,tenant_id,service_id,target_system_id,target_provider,desired_generation,created_by,updated_by)
			VALUES($1,$2,'vaultix',$3,'incus',1,'test','test')`, []any{uuid.NewString(), tenant, server}})
	h.Call("DELETE", base+"/vaultix", `{"reason":"retire it"}`, 409)
	h.Call("GET", base+"/vaultix", "", 200)

	if len(h.Audits) != 2 || h.Audits[1] != "shippin.services.catalog_service.delete catalogServiceId=backup · no longer sold" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 2 || h.Events[1].ID != "shippin.services.catalog_service.deleted" || h.Events[1].Subject["catalogServiceId"] != "backup" {
		t.Fatalf("events: %+v", h.Events)
	}
}
