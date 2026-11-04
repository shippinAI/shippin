package db

import "testing"

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
