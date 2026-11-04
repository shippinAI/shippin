package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
)

// Ops overrides of config keys: validated, audited with the write, cleared back to the config value.
func TestPlatformSettings(t *testing.T) {
	a, d, _ := notifyAPI(t)
	ctx := t.Context()
	_, _ = d.Pool.Exec(ctx, `DELETE FROM platform_settings`)
	t.Cleanup(func() { _, _ = d.Pool.Exec(ctx, `DELETE FROM platform_settings`) })
	a.cfg.IDP.ClientID = "client"
	a.cfg.IDP.Issuer, a.cfg.IDP.AccountPath = "https://idp.example.test", "/ui/console/users/me"
	a.cfg.Services.RRFlow.Enabled, a.cfg.Services.RRFlow.Tenants, a.cfg.Services.RRFlow.AutoAttach = true, []string{"t1"}, false
	a.cfg.Approval = approval.Policy{Enabled: true, Rules: []approval.Rule{{
		ID: "lifecycle-review", Actions: []string{hvAction}, ApproverRoles: []string{"op_infra"},
	}}}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO platform_settings(key,value,updated_by) VALUES('approval.hv_board','false','legacy')
		ON CONFLICT (key) DO UPDATE SET value=excluded.value,updated_by=excluded.updated_by`); err != nil {
		t.Fatal(err)
	}
	c := &caller{id: &auth.Identity{Sub: "ops-" + newUUID()}}
	put := func(key, body string) int {
		r := httptest.NewRequest("PUT", "/x", strings.NewReader(body))
		r.SetPathValue("key", key)
		w := httptest.NewRecorder()
		a.opSetPlatformSetting(w, r, c)
		return w.Code
	}
	audits := func() (n int) {
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE actor=$1 AND action='platform.setting'`, c.id.Sub).Scan(&n)
		return
	}
	if a.idpAccountURL(ctx) != "https://idp.example.test/ui/console/users/me" || a.rrflowAutoAttach(ctx, "t1") {
		t.Fatal("config defaults not used")
	}
	for _, bad := range []struct{ k, b string }{{"idp.account_path", `{"value":"https://evil.test/x","reason":"test change"}`},
		{"idp.account_path", `{"value":true,"reason":"test change"}`}, {"nope", `{"value":1,"reason":"test change"}`},
		{"approval.hv_board", `{"value":false,"reason":"disable approval"}`},
		{"services.rrflow.auto_attach", `{"value":true}`},
		{"services.probes", `{"value":[{"name":"Nomen","kind":"nomen","url":"file:///etc/passwd"}],"reason":"bad target"}`},
		{"services.probes", `{"value":[{"name":"Nomen","kind":"nomen","url":"https://user:pass@example.test/health"}],"reason":"bad credentials"}`},
		{"services.probes", `{"value":[{"name":"Nomen","kind":"nomen","url":"https://one.test/health","timeout_seconds":11}],"reason":"timeout too long"}`},
		{"services.probes", `{"value":[{"name":"Nomen","kind":"nomen","url":"https://one.test"},{"name":"nomen","kind":"nomen","tcp":"host:5432"}],"reason":"duplicate names"}`}} {
		if code := put(bad.k, bad.b); code < 400 {
			t.Fatalf("%s %s accepted (%d)", bad.k, bad.b, code)
		}
	}
	if audits() != 0 {
		t.Fatal("refused writes were audited")
	}
	if put("idp.account_path", `{"value":"/ui/v2/account","reason":"move to v2 UI"}`) != 200 ||
		put("services.rrflow.auto_attach", `{"value":true,"reason":"pilot tenant"}`) != 200 {
		t.Fatal("valid writes refused")
	}
	if a.idpAccountURL(ctx) != "https://idp.example.test/ui/v2/account" || !a.rrflowAutoAttach(ctx, "t1") || a.rrflowAutoAttach(ctx, "t2") {
		t.Fatal("overrides not effective (or auto-attach ignored the rrflow rollout)")
	}
	if put("idp.account_path", `{"value":null,"reason":"back to config"}`) != 200 || a.idpAccountURL(ctx) != "https://idp.example.test/ui/console/users/me" {
		t.Fatal("clearing did not fall back to the config value")
	}
	if audits() != 3 {
		t.Fatalf("want 3 audit rows, got %d", audits())
	}
}

func TestAgentHubConnectionConfigurationIsSealedAuditedAndWriteOnly(t *testing.T) {
	a, d, _ := notifyAPI(t)
	a.cfg.SessionKey = strings.Repeat("ab", 32)
	ctx := t.Context()
	_, _ = d.Pool.Exec(ctx, `DELETE FROM shippin_agenthub_connection`)
	c := &caller{id: &auth.Identity{Sub: "ops-" + newUUID()}}
	put := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "/v1/operator/agenthub-connection", strings.NewReader(body))
		w := httptest.NewRecorder()
		a.opSetAgentHubConnection(w, r, c)
		return w
	}
	for _, body := range []string{
		`{"base_url":"http://review.example.test","credential":"secret-token","reason":"bad protocol"}`,
		`{"base_url":"https://user:pass@review.example.test","reason":"userinfo rejected"}`,
		`{"base_url":"https://review.example.test/?token=x","reason":"query rejected"}`,
	} {
		if w := put(body); w.Code < 400 {
			t.Fatalf("unsafe endpoint accepted: %s", body)
		}
	}
	const token = "high-entropy-service-token"
	w := put(`{"base_url":"https://review.example.test/agenthub","credential":"` + token + `","reason":"configure review service"}`)
	if w.Code != 200 || strings.Contains(w.Body.String(), token) {
		t.Fatalf("save status=%d response=%s", w.Code, w.Body.String())
	}
	item, err := d.AgentHubConnection(ctx)
	if err != nil || !item.HasCredential || string(item.TokenCiphertext) == token {
		t.Fatalf("credential not stored as ciphertext: %#v err=%v", item, err)
	}
	plain, err := a.agentHubToken(ctx)
	if err != nil || plain != token {
		t.Fatalf("sealed credential did not resolve: %q err=%v", plain, err)
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response["has_credential"] != true {
		t.Fatalf("write-only response malformed: %v err=%v", response, err)
	}
	var audits int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE actor=$1 AND action='platform.agenthub.connection'`, c.id.Sub).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("expected one redacted audit row, got %d err=%v", audits, err)
	}
}
