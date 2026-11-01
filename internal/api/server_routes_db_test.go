package api

import (
	"context"
	"strings"
	"testing"
)

// TestServerAdminAuthz: only org owners/admins reach the server API; members get 403 everywhere.
func TestServerAdminAuthz(t *testing.T) {
	_, _, tn, srv := serverTestAPI(t, "org_member")
	base := "/v1/tenants/" + tn.ID + "/servers"
	for _, c := range [][2]string{{"GET", base}, {"POST", base}, {"POST", base + "/x/enroll-token"}, {"POST", base + "/x/revoke"}} {
		if code, _, _ := srvCall(t, srv, c[0], c[1], `{}`, ""); code != 403 {
			t.Errorf("%s %s as member: %d, want 403", c[0], c[1], code)
		}
	}
}

// TestServerAdminAPI: create/validate/cap, token shown once and never stored or audited in clear,
// estate rows read-only, revoke.
func TestServerAdminAPI(t *testing.T) {
	_, d, tn, srv := serverTestAPI(t, "org_owner")
	ctx := context.Background()
	base := "/v1/tenants/" + tn.ID + "/servers"
	for body, want := range map[string]int{`nope`: 400, `{"slug":"Gold","label":"g","site":"gold"}`: 400,
		`{"slug":"gold","label":"","site":"gold"}`: 400, `{"slug":"gold","label":"a\nb","site":"gold"}`: 400} {
		if code, _, _ := srvCall(t, srv, "POST", base, body, ""); code != want {
			t.Errorf("%s: %d, want %d", body, code, want)
		}
	}
	code, g, _ := srvCall(t, srv, "POST", base, `{"slug":"gold","label":"Gold (home)","site":"gold"}`, "")
	if code != 201 || g["state"] != "pending" || g["estate"] != false {
		t.Fatalf("create: %d %v", code, g)
	}
	if code, _, _ := srvCall(t, srv, "POST", base, `{"slug":"gold","label":"again","site":"gold"}`, ""); code != 409 {
		t.Fatalf("duplicate slug: %d", code)
	}
	id := g["id"].(string)
	code, tok, hdr := srvCall(t, srv, "POST", base+"/"+id+"/enroll-token", ``, "")
	plain, _ := tok["token"].(string)
	if code != 201 || !strings.HasPrefix(plain, "shpe_") || tok["expires_at"] == nil || hdr.Get("Cache-Control") != "no-store" {
		t.Fatalf("token: %d %v", code, hdr)
	}
	var leaks int
	_ = d.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_log WHERE detail LIKE '%'||$1||'%')
		+ (SELECT count(*) FROM server_enroll_tokens WHERE token_sha256=$1)
		+ (SELECT count(*) FROM idempotency_keys WHERE convert_from(coalesce(body,''::bytea),'UTF8') LIKE '%'||$1||'%')`, plain).Scan(&leaks)
	if leaks != 0 {
		t.Fatalf("token stored in clear %d times", leaks)
	}
	_, list, _ := srvCall(t, srv, "GET", base, "", "")
	planned := 0
	for _, it := range list["items"].([]any) {
		m := it.(map[string]any)
		if m["estate"] == true && m["state"] == "planned" {
			planned++
			if c, _, _ := srvCall(t, srv, "POST", base+"/"+m["id"].(string)+"/enroll-token", ``, ""); c != 403 {
				t.Errorf("token for planned %s: %d", m["slug"], c)
			}
			if c, _, _ := srvCall(t, srv, "POST", base+"/"+m["id"].(string)+"/revoke", ``, ""); c != 403 {
				t.Errorf("revoke planned %s: %d", m["slug"], c)
			}
		}
	}
	if planned != 3 {
		t.Fatalf("want Texas, Wisconsin, Virginia planned; got %d", planned)
	}
	if c, r, _ := srvCall(t, srv, "POST", base+"/"+id+"/revoke", ``, ""); c != 200 || r["state"] != "revoked" {
		t.Fatalf("revoke: %d %v", c, r)
	}
	if c, _, _ := srvCall(t, srv, "POST", base+"/"+id+"/revoke", ``, ""); c != 409 {
		t.Fatalf("revoke twice: %d", c)
	}
	if c, _, _ := srvCall(t, srv, "POST", "/v1/servers/enroll", `{"token":"`+plain+`"}`, ""); c != 401 {
		t.Fatalf("token of a revoked server: %d", c)
	}
}
