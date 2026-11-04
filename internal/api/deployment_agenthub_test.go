package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeploymentReviewEvidenceRedactsSecretReferences(t *testing.T) {
	action := deploymentReviewAction{
		Operation: "deployment.present", TenantID: "tenant-1", DeploymentID: "deployment-1", Generation: 3,
		TargetSystemID: "system-1", TargetProvider: "proxmox", ServiceID: "postgres",
		DefinitionSHA: strings.Repeat("a", 64),
		Source:        map[string]string{"kind": "oci", "reference": "registry.example/postgres@sha256:deadbeef"},
		Config:        json.RawMessage(`{"database":"app","auth":{"password":"vaultix://tenant/project/database"},"tokens":["sops://runtime/secrets.yaml#/token"]}`),
	}
	text, err := deploymentReviewEvidence(action)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "vaultix://") || strings.Contains(text, "sops://") || !strings.Contains(text, `"database":"app"`) {
		t.Fatalf("review evidence was not useful and secret-reference redacted: %s", text)
	}
}

func TestDeploymentReviewEvidenceRejectsUnboundedArtifact(t *testing.T) {
	action := deploymentReviewAction{Config: json.RawMessage(`{"value":"` + strings.Repeat("x", 21_000) + `"}`)}
	if _, err := deploymentReviewEvidence(action); err == nil {
		t.Fatal("oversized review evidence was accepted")
	}
}
