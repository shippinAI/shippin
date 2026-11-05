package catalog

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogServiceAPIValidatesRuntimeRecipe(t *testing.T) {
	base := `{"id":"postgres","label":"Postgres","kind":"database","config_schema":{"type":"object","properties":{},"additionalProperties":false},"default_config":{},"deployment_source":{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"runtime_contract":%s}`
	valid := `{"version":2,"kind":"incus_oci_container","providers":["incus"],"resources":{"cpu_cores":2,"memory_mib":2048},"storage":{"size_gib":20,"mount_path":"/var/lib/postgresql/data"},"network":{"exposure":"private","ports":[{"name":"postgres","protocol":"tcp","container_port":5432}]},"health":{"kind":"tcp","port":5432,"interval_seconds":30,"timeout_seconds":5,"failure_threshold":3}}`

	handler := &Handler{}
	read := func(body string) (Service, bool) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/v1/operator/catalog/services", strings.NewReader(body))
		return handler.readService(recorder, request)
	}
	service, ok := read(strings.Replace(base, "%s", valid, 1))
	if !ok || service.RuntimeContract == nil || service.RuntimeContract.Version != 2 {
		t.Fatalf("valid runtime recipe rejected: ok=%v contract=%+v", ok, service.RuntimeContract)
	}

	invalid := []string{
		strings.Replace(valid, `"cpu_cores":2`, `"cpu_cores":0`, 1),
		strings.Replace(valid, `"mount_path":"/var/lib/postgresql/data"`, `"mount_path":"/var/lib/../etc"`, 1),
		strings.Replace(valid, `"exposure":"private"`, `"exposure":"public"`, 1),
		strings.Replace(valid, `"port":5432`, `"port":3306`, 1),
		strings.Replace(valid, `"timeout_seconds":5`, `"timeout_seconds":30`, 1),
	}
	for index, recipe := range invalid {
		t.Run(fmt.Sprintf("invalid_%d", index+1), func(t *testing.T) {
			if _, ok := read(strings.Replace(base, "%s", recipe, 1)); ok {
				t.Fatal("invalid runtime recipe accepted")
			}
		})
	}
}
