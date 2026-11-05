package deployments

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/catalog"
)

func executionPlanFixture(t *testing.T) (*Deployment, string) {
	t.Helper()
	request := postgresRequest()
	request.Definition.Runtime = catalog.RuntimeContract{
		Version: 3, Kind: "incus_oci_container", Providers: []string{"incus"},
		Resources:   &catalog.RuntimeResources{CPUCores: 2, MemoryMiB: 2048},
		Storage:     &catalog.RuntimeStorage{SizeGiB: 20, MountPath: "/var/lib/postgresql/data"},
		Network:     &catalog.RuntimeNetwork{Exposure: "private", Ports: []catalog.RuntimePort{{Name: "postgres", Protocol: "tcp", ContainerPort: 5432}}},
		Health:      &catalog.RuntimeHealthCheck{Kind: "tcp", Port: 5432, IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
		Environment: &[]catalog.RuntimeEnvironmentBinding{{Name: "POSTGRES_PASSWORD", ConfigPath: "/password"}},
	}
	request.TargetConfig = json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`)
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	definitionSHA, err := request.Definition.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(request.Definition)
	if err != nil {
		t.Fatal(err)
	}
	target, err := normalizeTargetConfig(request.TargetConfig, request.TargetProvider, request.Definition.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	deployment := &Deployment{
		ID: uuid.NewString(), ServiceID: request.ServiceID, TargetSystemID: request.TargetSystemID,
		TargetProvider: request.TargetProvider, Generation: 1, DesiredState: DesiredPresent,
		DefinitionSHA256: definitionSHA, Config: request.Config, TargetConfig: TargetConfig{},
		definitionSnapshot: snapshot,
	}
	if err := json.Unmarshal(target, &deployment.TargetConfig); err != nil {
		t.Fatal(err)
	}
	return deployment, request.TenantID
}

func TestExecutionPlanBindsImmutableGenerationAndExactStepSet(t *testing.T) {
	deployment, tenantID := executionPlanFixture(t)
	plan, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version != ExecutionPlanVersion || plan.UnsupportedReason != "executor_not_installed" || len(plan.Steps) != 5 {
		t.Fatalf("unexpected Incus runtime plan: %+v", plan)
	}
	if got := []string{plan.Steps[0].Operation, plan.Steps[1].Operation, plan.Steps[2].Operation, plan.Steps[3].Operation, plan.Steps[4].Operation}; strings.Join(got, ",") != "ensure_persistent_volume,ensure_oci_instance,ensure_running,verify_instance_identity,verify_declared_health" {
		t.Fatalf("provider step order = %v", got)
	}
	baseline, err := plan.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}

	mutations := map[string]func(*ExecutionPlan){
		"target":     func(p *ExecutionPlan) { p.TargetSystemID = "019a0000-0000-7000-8000-000000000099" },
		"generation": func(p *ExecutionPlan) { p.Generation++ },
		"source":     func(p *ExecutionPlan) { p.SourceSHA256 = strings.Repeat("a", 64) },
		"config":     func(p *ExecutionPlan) { p.ConfigSHA256 = strings.Repeat("b", 64) },
		"target config": func(p *ExecutionPlan) {
			p.TargetConfigSHA256 = strings.Repeat("c", 64)
		},
		"step set": func(p *ExecutionPlan) {
			p.Steps = append(p.Steps, ExecutionStep{ID: "approval-extra", Operation: "unexpected_write", IdempotencyKey: strings.Repeat("d", 64)})
		},
		"step order": func(p *ExecutionPlan) { p.Steps[0], p.Steps[1] = p.Steps[1], p.Steps[0] },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := plan
			changed.Steps = append([]ExecutionStep(nil), plan.Steps...)
			mutate(&changed)
			fingerprint, err := changed.Fingerprint()
			if err != nil {
				t.Fatal(err)
			}
			if fingerprint == baseline {
				t.Fatalf("changing %s did not change the execution-plan digest", name)
			}
		})
	}
	unsupportedVersion := plan
	unsupportedVersion.Version++
	if _, err := unsupportedVersion.Fingerprint(); err == nil {
		t.Fatal("an unknown execution-plan version was accepted")
	}
}

func TestExecutionPlanCanonicalConfigAndDeterministicStepKeys(t *testing.T) {
	deployment, tenantID := executionPlanFixture(t)
	first, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		t.Fatal(err)
	}
	firstHash, err := first.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	deployment.Config = json.RawMessage(`{"password":"vaultix://production/postgres/password","port":5432}`)
	second, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := second.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatal("JSON object key order changed the execution-plan digest")
	}
	encoded, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "vaultix://") || strings.Contains(string(encoded), "postgres") && strings.Contains(string(encoded), "password") {
		t.Fatalf("plan copied deployment config or secret reference: %s", encoded)
	}
	key, err := StepIdempotencyKey(tenantID, deployment.ID, deployment.Generation, ExecutionPlanVersion,
		ProviderIncus, DesiredPresent, "workload", "ensure_oci_instance")
	if err != nil {
		t.Fatal(err)
	}
	keyAgain, err := StepIdempotencyKey(tenantID, deployment.ID, deployment.Generation, ExecutionPlanVersion,
		ProviderIncus, DesiredPresent, "workload", "ensure_oci_instance")
	if err != nil || key != keyAgain || len(key) != 64 {
		t.Fatalf("step idempotency key is not stable: %q %q %v", key, keyAgain, err)
	}
	removeKey, err := StepIdempotencyKey(tenantID, deployment.ID, deployment.Generation, ExecutionPlanVersion,
		ProviderIncus, DesiredAbsent, "workload", "delete_owned_instance_preserve_volume")
	if err != nil || key == removeKey {
		t.Fatalf("step key did not bind operation and desired state: create=%q remove=%q err=%v", key, removeKey, err)
	}
}

func TestExecutionPlanRemovePreservesDeploymentVolume(t *testing.T) {
	deployment, tenantID := executionPlanFixture(t)
	deployment.DesiredState = DesiredAbsent
	plan, err := BuildExecutionPlan(tenantID, deployment)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[1].Operation != "delete_owned_instance_preserve_volume" {
		t.Fatalf("removal plan does not preserve persistent data: %+v", plan.Steps)
	}
}
