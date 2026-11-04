package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hv"
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
	a.cfg.Approval = approval.Policy{Enabled: true, Board: approval.BoardConfig{Driver: "hub"}, Rules: []approval.Rule{{
		ID: "lifecycle-review", Actions: []string{hv.ActionLifecycle}, ApproverRoles: []string{"op_infra"}, NeedsApproval: true,
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
	if !a.hvNeedsBoard(ctx, hv.ActionLifecycle, "configured-host") {
		t.Fatal("legacy UI override must not disable configured lifecycle review")
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
		put("services.rrflow.auto_attach", `{"value":true,"reason":"pilot tenant"}`) != 200 ||
		put("services.probes", `{"value":[{"name":"Nomen","kind":"nomen","instance":"nomen","url":"https://nomen.example.test/health","timeout_seconds":5},{"name":"Vaultix","kind":"vaultix","instance":"vaultix","tcp":"vaultix.internal:8080","timeout_seconds":2}],"reason":"configure platform health targets"}`) != 200 {
		t.Fatal("valid writes refused")
	}
	probes := a.platformProbes(ctx)
	if len(probes) != 2 || probes[0].URL != "https://nomen.example.test/health" || probes[0].TimeoutSeconds != 5 || probes[1].TCP != "vaultix.internal:8080" || probes[1].TimeoutSeconds != 2 {
		t.Fatalf("saved probes did not feed live service health: %#v", probes)
	}
	if a.idpAccountURL(ctx) != "https://idp.example.test/ui/v2/account" || !a.rrflowAutoAttach(ctx, "t1") || a.rrflowAutoAttach(ctx, "t2") {
		t.Fatal("overrides not effective (or auto-attach ignored the rrflow rollout)")
	}
	if put("idp.account_path", `{"value":null,"reason":"back to config"}`) != 200 || a.idpAccountURL(ctx) != "https://idp.example.test/ui/console/users/me" {
		t.Fatal("clearing did not fall back to the config value")
	}
	if audits() != 4 {
		t.Fatalf("want 4 audit rows, got %d", audits())
	}
}
