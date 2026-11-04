package db

import (
	"github.com/google/uuid"
	"testing"
)

func TestAgentHubConnectionStoresOnlyOpaqueCredentialAndAudits(t *testing.T) {
	d, ctx := testDB(t)
	ciphertext := []byte("sealed-test-value")
	audit := AuditEntry{Actor: "operator", ActorName: "Operator", Action: "platform.agenthub.connection", Detail: "credential replaced"}
	if err := d.SaveAgentHubConnection(ctx, "https://review.example.test", ciphertext, false, "operator", audit); err != nil {
		t.Fatal(err)
	}
	got, err := d.AgentHubConnection(ctx)
	if err != nil || got.BaseURL != "https://review.example.test" || !got.HasCredential || string(got.TokenCiphertext) != string(ciphertext) {
		t.Fatalf("connection=%#v err=%v", got, err)
	}
	if err := d.SaveAgentHubConnection(ctx, "https://review.example.test/path", nil, true, "operator"); err != nil {
		t.Fatal(err)
	}
	got, err = d.AgentHubConnection(ctx)
	if err != nil || string(got.TokenCiphertext) != string(ciphertext) || got.BaseURL != "https://review.example.test/path" {
		t.Fatalf("credential was not retained on endpoint update: %#v err=%v", got, err)
	}
	if err := d.SaveAgentHubConnection(ctx, "https://review.example.test/path", nil, false, "operator"); err != nil {
		t.Fatal(err)
	}
	got, err = d.AgentHubConnection(ctx)
	if err != nil || got.HasCredential || len(got.TokenCiphertext) != 0 {
		t.Fatalf("credential clear failed: %#v err=%v", got, err)
	}
	var audits int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action='platform.agenthub.connection' AND actor='operator'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit rows=%d err=%v", audits, err)
	}
}

func TestAgentHubMappingChecksAreAppendOnlyAndAudited(t *testing.T) {
	d, ctx := testDB(t)
	tenantID, serverID := uuid.NewString(), uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES($1,$2,'mapping test','starter','active')`, tenantID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hostname,hostname_source)
		VALUES($1,$2,'mapping-test','mapping-test','test','planned','operator','mapping-test','hostd_enrollment')`, serverID, tenantID); err != nil {
		t.Fatal(err)
	}
	version := int64(3)
	actor := "agenthub-map-" + uuid.NewString()
	check := AgentHubMappingCheck{ID: uuid.NewString(), ServerID: serverID, TenantID: "tenant-1", MachineID: "machine-1", Outcome: "verified", PolicyVersion: &version,
		PolicySHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Detail: "mapping verified", CheckedBy: actor}
	audit := AuditEntry{Actor: actor, ActorName: "Operator", Action: "infra.agenthub.mapping.check", Detail: "mapping result recorded"}
	got, err := d.RecordAgentHubMappingCheck(ctx, check, audit)
	if err != nil || got.CheckedAt.IsZero() {
		t.Fatalf("recorded check=%+v err=%v", got, err)
	}
	check.ID, check.Outcome, check.PolicyVersion, check.PolicySHA256 = uuid.NewString(), "failed", nil, ""
	check.Detail = "mapping could not be verified"
	if _, err := d.RecordAgentHubMappingCheck(ctx, check, audit); err != nil {
		t.Fatal(err)
	}
	var checks, audits int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_agenthub_mapping_checks WHERE server_id=$1`, serverID).Scan(&checks); err != nil || checks != 2 {
		t.Fatalf("mapping observations=%d err=%v", checks, err)
	}
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action='infra.agenthub.mapping.check' AND actor=$1`, actor).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("mapping audits=%d err=%v", audits, err)
	}
}
