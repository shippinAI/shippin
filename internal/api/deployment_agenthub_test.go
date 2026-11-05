package api

import (
	"encoding/json"
	"strings"
	"testing"

	"shippin.cloud/console/internal/deployments"
)

func TestDeploymentReviewEvidenceRedactsSecretReferences(t *testing.T) {
	action := deploymentReviewAction{
		Operation: "deployment.present", TenantID: "tenant-1", DeploymentID: "deployment-1", Generation: 3,
		TargetSystemID: "system-1", TargetProvider: "proxmox", ServiceID: "postgres",
		DefinitionSHA: strings.Repeat("a", 64),
		Source:        deployments.SourceView{Kind: "oci", Reference: "registry.example/postgres@sha256:deadbeef"},
		Config:        json.RawMessage(`{"database":"app","auth":{"password":"vaultix://production/database/password"},"tokens":["sops://runtime/secrets.yaml#/token"]}`),
	}
	text, err := deploymentReviewEvidence(action, "shp-review-123")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "vaultix://") || strings.Contains(text, "sops://") || !strings.Contains(text, `"database":"app"`) ||
		!strings.Contains(text, `"ticket_id":"shp-review-123"`) {
		t.Fatalf("review evidence was not useful and secret-reference redacted: %s", text)
	}
}

func TestDeploymentReviewEvidenceRejectsUnboundedArtifact(t *testing.T) {
	action := deploymentReviewAction{Config: json.RawMessage(`{"value":"` + strings.Repeat("x", 21_000) + `"}`)}
	if _, err := deploymentReviewEvidence(action, "shp-review-123"); err == nil {
		t.Fatal("oversized review evidence was accepted")
	}
}

func TestDeploymentReviewContextSatisfiesAgentHubReviewFields(t *testing.T) {
	action := deploymentReviewAction{
		TenantID: "tenant-1", DeploymentID: "deployment-1", Generation: 3, TargetSystemID: "system-1",
		TargetProvider: "incus", ServiceID: "postgres",
		ExecutionPlanSHA256: strings.Repeat("b", 64),
		ExecutionPlan:       deployments.ExecutionPlan{Version: deployments.ExecutionPlanVersion},
		Source:              deployments.SourceView{Kind: "github", Reference: "https://github.com/example/postgres", Commit: strings.Repeat("a", 40)},
	}
	path := "/shippin/deployments/deployment-1/generations/3/review.json"
	fields := deploymentReviewFields(action, "shp-review-123", path, strings.Repeat("b", 64))
	for _, key := range []string{"topology", "blast_radius", "snapshot", "shippin_action_sha256", "shippin_execution_plan_sha256", "shippin_execution_plan_version", "shippin_agenthub_ticket_id", "shippin_system_id", "shippin_deployment_id", "shippin_generation"} {
		if fields[key] == "" {
			t.Errorf("AgentHub review field %q is missing", key)
		}
	}
	if fields["snapshot"] != path || len(fields["topology"]) < 10 || len(fields["blast_radius"]) < 20 {
		t.Fatalf("AgentHub required context fields are incomplete: %#v", fields)
	}
	lists := deploymentReviewLists(action, path)
	if len(lists["research"]) != 1 || lists["research"][0] != action.Source.Reference || len(lists["verification"]) < 1 {
		t.Fatalf("AgentHub research and verification lists are incomplete: %#v", lists)
	}
	action.Source = deployments.SourceView{Kind: "oci", Reference: "registry.example/postgres@sha256:" + strings.Repeat("a", 64)}
	if got := deploymentReviewLists(action, path)["research"]; len(got) != 1 || got[0] != path {
		t.Fatalf("non-URL source must use attached snapshot as research evidence: %#v", got)
	}
}
