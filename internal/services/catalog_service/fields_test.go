package catalog_service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// read decodes a body strictly, as the action runner does, and normalizes it.
func read(body string) (CreateInput, error) {
	var in CreateInput
	dec := json.NewDecoder(bytes.NewReader([]byte(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return in, err
	}
	return in, in.normalize(in.ID)
}

// Ported from the legacy TestCatalogServiceAPIValidatesRuntimeRecipe.
func TestFieldsValidateRuntimeRecipe(t *testing.T) {
	base := `{"id":"postgres","label":"Postgres","kind":"database","config_schema":{"type":"object","properties":{},"additionalProperties":false},"default_config":{},"deployment_source":{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"runtime_contract":%s}`
	valid := `{"version":2,"kind":"incus_oci_container","providers":["incus"],"resources":{"cpu_cores":2,"memory_mib":2048},"storage":{"size_gib":20,"mount_path":"/var/lib/postgresql/data"},"network":{"exposure":"private","ports":[{"name":"postgres","protocol":"tcp","container_port":5432}]},"health":{"kind":"tcp","port":5432,"interval_seconds":30,"timeout_seconds":5,"failure_threshold":3}}`
	in, err := read(strings.Replace(base, "%s", valid, 1))
	if err != nil || in.RuntimeContract == nil || in.RuntimeContract.Version != 2 {
		t.Fatalf("valid runtime recipe rejected: %v contract=%+v", err, in.RuntimeContract)
	}
	invalid := []string{
		strings.Replace(valid, `"cpu_cores":2`, `"cpu_cores":0`, 1),
		strings.Replace(valid, `"mount_path":"/var/lib/postgresql/data"`, `"mount_path":"/var/lib/../etc"`, 1),
		strings.Replace(valid, `"exposure":"private"`, `"exposure":"public"`, 1),
		strings.Replace(valid, `"port":5432`, `"port":3306`, 1),
		strings.Replace(valid, `"timeout_seconds":5`, `"timeout_seconds":30`, 1),
	}
	for i, recipe := range invalid {
		t.Run(fmt.Sprintf("invalid_%d", i+1), func(t *testing.T) {
			if _, err := read(strings.Replace(base, "%s", recipe, 1)); err == nil || !strings.HasPrefix(err.Error(), "The runtime contract is invalid: ") {
				t.Fatalf("invalid runtime recipe accepted or misreported: %v", err)
			}
		})
	}
	v3 := `{"version":3,"kind":"incus_oci_container","providers":["incus"],"resources":{"cpu_cores":2,"memory_mib":2048},"storage":{"size_gib":20,"mount_path":"/var/lib/postgresql/data"},"network":{"exposure":"private","ports":[{"name":"postgres","protocol":"tcp","container_port":5432}]},"health":{"kind":"tcp","port":5432,"interval_seconds":30,"timeout_seconds":5,"failure_threshold":3},"environment":[{"name":"POSTGRES_DB","config_path":"/database"}]}`
	v3Body := `{"id":"postgres","label":"Postgres","kind":"database","config_schema":{"type":"object","properties":{"database":{"type":"string"}},"required":["database"],"additionalProperties":false},"default_config":{"database":"app"},"deployment_source":{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"runtime_contract":` + v3 + `}`
	if in, err = read(v3Body); err != nil || in.RuntimeContract == nil || in.RuntimeContract.Version != 3 {
		t.Fatalf("valid v3 runtime bindings rejected: %v contract=%+v", err, in.RuntimeContract)
	}
	if _, err := read(strings.Replace(v3Body, `"config_path":"/database"`, `"config_path":"/missing"`, 1)); err == nil ||
		!strings.HasPrefix(err.Error(), "The runtime environment bindings are invalid: ") {
		t.Fatalf("runtime binding to an undeclared config field accepted or misreported: %v", err)
	}
}

// Config schema and defaults are JSON objects whose values keep their exact JSON; anything else is refused
// with a message that says so.
func TestFieldsObjects(t *testing.T) {
	in, err := read(`{"id":"big","label":"Big","default_config":{"n":12345678901234567890,"s":"x"}}`)
	if err != nil || string(in.DefaultConfig["n"]) != "12345678901234567890" || string(in.ConfigSchema.raw()) != "{}" || in.Kind != "app" || !boolOr(in.Active) {
		t.Fatalf("defaults or exact values lost: %v %s %+v", err, in.DefaultConfig["n"], in)
	}
	for _, bad := range []string{`"config_schema":null`, `"default_config":[]`, `"default_config":"x"`} {
		if _, err := read(`{"id":"bad","label":"Bad",` + bad + `}`); err == nil || err.Error() != errNotObject.Error() {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	for _, bad := range []string{`{"id":"Bad","label":"x"}`, `{"id":"ok","label":""}`} {
		if _, err := read(bad); err == nil || err.Error() != "A service needs a lowercase id, a label, and JSON objects for config schema and defaults." {
			t.Fatalf("%s: %v", bad, err)
		}
	}
}
