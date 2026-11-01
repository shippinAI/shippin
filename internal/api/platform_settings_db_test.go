package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
)

// Ops overrides of config keys: validated, audited with the write, cleared back to the config value.
func TestPlatformSettings(t *testing.T) {
	a, d, _ := notifyAPI(t)
	ctx := t.Context()
	_, _ = d.Pool.Exec(ctx, `DELETE FROM platform_settings`)
	t.Cleanup(func() { _, _ = d.Pool.Exec(ctx, `DELETE FROM platform_settings`) })
	a.cfg.IDP.Issuer, a.cfg.IDP.AccountPath = "https://idp.example.test", "/ui/console/users/me"
	a.cfg.Services.RRFlow.Enabled, a.cfg.Services.RRFlow.Tenants, a.cfg.Services.RRFlow.AutoAttach = true, []string{"t1"}, false
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
		{"services.rrflow.auto_attach", `{"value":true}`}} {
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
