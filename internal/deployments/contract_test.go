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
