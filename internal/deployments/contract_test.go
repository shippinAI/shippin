package deployments

import (
	"encoding/json"
	"testing"

	"shippin.cloud/console/internal/catalog"
)

func postgresRequest() DeploymentRequest {
	return DeploymentRequest{
		TenantID:       "019a0000-0000-7000-8000-000000000001",
		ServiceID:      "postgres",
		TargetSystemID: "019a0000-0000-7000-8000-000000000002",
		TargetProvider: ProviderIncus,
		DesiredState:   DesiredPresent,
		IdempotencyKey: "019a0000-0000-7000-8000-000000000003",
		Actor:          "operator@example.test",
		Definition: DefinitionSnapshot{
			ServiceID: "postgres",
			Source: catalog.DeploymentSource{
				Kind:      "oci",
				Reference: "registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Runtime:       catalog.RuntimeContract{Version: 1, Kind: "incus_oci_container", Providers: []string{"incus"}},
			ConfigSchema:  json.RawMessage(`{"type":"object","properties":{"port":{"type":"integer"},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false}`),
			DefaultConfig: json.RawMessage(`{"port":5432,"password":"vaultix://production/postgres/password"}`),
		},
		Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/postgres/password"}`),
	}
}

func TestDeploymentRequestBindsTenantTargetPinnedDefinitionAndSecretSafeConfig(t *testing.T) {
	request := postgresRequest()
	if err := request.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	fingerprint, err := request.RequestFingerprint()
	if err != nil || len(fingerprint) != 64 {
		t.Fatalf("request fingerprint = %q, %v", fingerprint, err)
	}
	definitionHash, err := request.Definition.Fingerprint()
	if err != nil || len(definitionHash) != 64 {
		t.Fatalf("definition fingerprint = %q, %v", definitionHash, err)
	}

	changed := request
	changed.TargetSystemID = "019a0000-0000-7000-8000-000000000004"
	changedFingerprint, err := changed.RequestFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint == changedFingerprint {
		t.Fatal("changing target must change the idempotent request fingerprint")
	}
}

func TestDeploymentRequestSnapshotsValidatedRuntimeRecipeV2(t *testing.T) {
	request := postgresRequest()
	request.Definition.Runtime = catalog.RuntimeContract{
		Version: 2, Kind: "incus_oci_container", Providers: []string{"incus"},
		Resources: &catalog.RuntimeResources{CPUCores: 4, MemoryMiB: 8192},
		Storage:   &catalog.RuntimeStorage{SizeGiB: 100, MountPath: "/var/lib/postgresql/data"},
		Network:   &catalog.RuntimeNetwork{Exposure: "private", Ports: []catalog.RuntimePort{{Name: "postgres", Protocol: "tcp", ContainerPort: 5432}}},
		Health:    &catalog.RuntimeHealthCheck{Kind: "tcp", Port: 5432, IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
	}
	request.TargetConfig = json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`)
	if err := request.Validate(); err != nil {
		t.Fatalf("valid recipe generation rejected: %v", err)
	}
	fingerprint, err := request.Definition.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	request.Definition.Runtime.Resources.CPUCores++
	changed, err := request.Definition.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint == changed {
		t.Fatal("runtime resource changes must be bound by the immutable definition fingerprint")
	}
}

func TestDeploymentRequestV3BindsOnlySchemaDeclaredRuntimeStrings(t *testing.T) {
	request := postgresRequest()
	environment := []catalog.RuntimeEnvironmentBinding{{Name: "POSTGRES_PASSWORD", ConfigPath: "/password"}}
	request.Definition.Runtime = catalog.RuntimeContract{
		Version: 3, Kind: "incus_oci_container", Providers: []string{"incus"},
		Resources:   &catalog.RuntimeResources{CPUCores: 2, MemoryMiB: 2048},
		Storage:     &catalog.RuntimeStorage{SizeGiB: 20, MountPath: "/var/lib/postgresql/data"},
		Network:     &catalog.RuntimeNetwork{Exposure: "private", Ports: []catalog.RuntimePort{{Name: "postgres", Protocol: "tcp", ContainerPort: 5432}}},
		Health:      &catalog.RuntimeHealthCheck{Kind: "tcp", Port: 5432, IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
		Environment: &environment,
	}
	request.TargetConfig = json.RawMessage(`{}`)
	if err := request.Validate(); err == nil {
		t.Fatal("v3 deployment accepted without the required Incus storage pool")
	}
	request.TargetConfig = json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`)
	if err := request.Validate(); err != nil {
		t.Fatalf("valid v3 deployment rejected: %v", err)
	}
	request.Config = json.RawMessage(`{"port":5432,"password":"sops://runtime/secrets.yaml#/postgres/password"}`)
	if err := request.Validate(); err == nil {
		t.Fatal("runtime binding accepted a secret provider without a deployment resolver")
	}
	request.Config = json.RawMessage(`{"port":5432,"password":"vaultix://production/postgres/password"}`)
	(*request.Definition.Runtime.Environment)[0].ConfigPath = "/port"
	if err := request.Validate(); err == nil {
		t.Fatal("runtime binding accepted a non-string schema field")
	}
}

func TestDeploymentTargetConfigRequiresExplicitCompatiblePool(t *testing.T) {
	request := postgresRequest()
	request.Definition.Runtime = catalog.RuntimeContract{
		Version: 2, Kind: "incus_oci_container", Providers: []string{"incus"},
		Resources: &catalog.RuntimeResources{CPUCores: 2, MemoryMiB: 2048},
		Storage:   &catalog.RuntimeStorage{SizeGiB: 20, MountPath: "/var/lib/data"},
		Network:   &catalog.RuntimeNetwork{Exposure: "private", Ports: []catalog.RuntimePort{{Name: "db", Protocol: "tcp", ContainerPort: 5432}}},
		Health:    &catalog.RuntimeHealthCheck{Kind: "tcp", Port: 5432, IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
	}
	for _, raw := range []string{"{}", `{"storage":{"pool":"tenant-data"}}`, `{"network":{"name":"private-apps"}}`,
		`{"storage":{"pool":""},"network":{"name":"private-apps"}}`,
		`{"storage":{"pool":"../other"},"network":{"name":"private-apps"}}`,
		`{"storage":{"pool":"tenant-data"},"network":{"name":"../other"}}`,
		`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps","host":"unexpected"}}`, `null`, `[]`} {
		t.Run(raw, func(t *testing.T) {
			request.TargetConfig = json.RawMessage(raw)
			if err := request.Validate(); err == nil {
				t.Fatalf("invalid target config accepted: %s", raw)
			}
		})
	}
	request.TargetConfig = json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`)
	if err := request.Validate(); err != nil {
		t.Fatalf("valid target-specific pool rejected: %v", err)
	}
	request.TargetConfig = json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`)
	request.TargetProvider = ProviderProxmox
	if err := request.Validate(); err == nil {
		t.Fatal("Incus pool accepted for an incompatible provider")
	}
}

func TestDeploymentRequestRejectsInvalidScopeProviderAndSecretValues(t *testing.T) {
	mutations := map[string]func(*DeploymentRequest){
		"invalid tenant":              func(r *DeploymentRequest) { r.TenantID = "tenant-a" },
		"unsupported provider":        func(r *DeploymentRequest) { r.TargetProvider = "docker" },
		"incompatible provider":       func(r *DeploymentRequest) { r.TargetProvider = ProviderProxmox },
		"unresolved password":         func(r *DeploymentRequest) { r.Config = json.RawMessage(`{"port":5432,"password":"plain-text"}`) },
		"unpaired service definition": func(r *DeploymentRequest) { r.Definition.ServiceID = "redis" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			request := postgresRequest()
			mutate(&request)
			if err := request.Validate(); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}

func TestDeploymentEventsAreTypedAndDoNotAcceptRawProviderErrors(t *testing.T) {
	valid := DeploymentEvent{
		ID: "019a0000-0000-7000-8000-000000000010", DeploymentID: "019a0000-0000-7000-8000-000000000011",
		TenantID: "019a0000-0000-7000-8000-000000000001", Generation: 1, Type: EventOperationFailed,
		ObservedState: ObservedFailed, FailureCode: "provider.timeout", Actor: "operator@example.test",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid normalized event rejected: %v", err)
	}

	for name, mutate := range map[string]func(*DeploymentEvent){
		"raw failure text":     func(e *DeploymentEvent) { e.FailureCode = "password=plain-text" },
		"URI in operation id":  func(e *DeploymentEvent) { e.ProviderOperationID = "https://user:password@example.test" },
		"failure without code": func(e *DeploymentEvent) { e.FailureCode = "" },
	} {
		t.Run(name, func(t *testing.T) {
			event := valid
			mutate(&event)
			if err := event.Validate(); err == nil {
				t.Fatal("unsafe event accepted")
			}
		})
	}
}
