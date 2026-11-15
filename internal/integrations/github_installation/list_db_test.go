package github_installation_test

import (
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const (
	acme   = "01a14149-0000-7000-8000-00000000000a"
	globex = "01a14149-0000-7000-8000-00000000000b"
)

func TestConnections(t *testing.T) {
	owner := func(p string) bool { return p == "decide" || p == "manage" }
	h := mcpltest.Serve(t, actions.All, "shippin.integrations.github_installation.", mcpltest.As{Actor: "ada", Tenant: acme, Perms: owner})
	if _, err := h.Pool.Exec(t.Context(), `
	  INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES('`+acme+`','unlinked:a','Acme','pro','active'), ('`+globex+`','unlinked:b','Globex','pro','active');
	  INSERT INTO github_installations(id,tenant_id,account_login,account_type,suspended_at) VALUES
	    (11,'`+acme+`','acme','Organization',NULL), (12,'`+acme+`','ada','User',now()), (22,'`+globex+`','globex','Organization',NULL);
	  INSERT INTO github_installations(id,tenant_id,account_login,account_type,deleted_at) VALUES (13,'`+acme+`','old','User',now())`); err != nil {
		t.Fatal(err)
	}
	base := "/v1/tenants/" + acme + "/integrations/github-installations"
	got := h.Call("GET", base, "", 200)
	items := got["items"].([]any)
	if len(items) != 2 || got["can_connect"] != true {
		t.Fatalf("Acme's live connections, owner may connect: %v", got)
	}
	if second := items[1].(map[string]any); second["account_login"] != "ada" || second["suspended"] != true {
		t.Fatalf("a suspended connection is shown as suspended: %v", second)
	}
	h.As.Perms = func(p string) bool { return false }
	if got := h.Call("GET", base, "", 200); got["can_connect"] != false {
		t.Fatalf("a member who is not an owner or admin may not connect: %v", got)
	}
	h.As.Tenant = globex
	if l := h.Call("GET", "/v1/tenants/"+globex+"/integrations/github-installations", "", 200)["items"].([]any); len(l) != 1 {
		t.Fatalf("Globex sees only its own: %v", l)
	}
}
