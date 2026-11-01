package api

import (
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

func TestPublishedSparkHostStillNeedsOwningTenant(t *testing.T) {
	a := gateAPI("op_superadmin")
	a.cfg.Operator.AdminHosts = []string{"spark.example.test", "mail.example.test"}
	a.cfg.Sparks = config.Sparks{TenantID: "owner", Nodes: []config.SparkNode{{Management: []config.SparkEndpoint{{State: "published", URL: "https://spark.example.test/lab"}}}}}
	r := httptest.NewRequest("GET", "/auth/ws-check", nil)
	for _, tc := range []struct {
		name, tenant, role string
		grant, want        bool
	}{
		{"owner admin", "owner", "op_superadmin", false, true},
		{"other admin", "other", "op_superadmin", false, false},
		{"support grant", "owner", "op_superadmin", true, false},
		{"member only", "owner", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := tenantView{Tenant: db.Tenant{ID: tc.tenant}}
			if tc.grant {
				v.grants = []db.Grant{{ID: "grant"}}
			}
			c := &caller{id: testID("u1"), op: tc.role, tenants: []tenantView{v}}
			if got := a.sparkPermit(r, c, "spark.example.test"); got != tc.want {
				t.Fatalf("allowed=%v want=%v", got, tc.want)
			}
		})
	}
	if a.sparkHost("spark.example.test.evil.test") || a.sparkHost("mail.example.test") {
		t.Fatal("unconfigured host matched Spark access")
	}
	if !a.adminHost("mail.example.test") {
		t.Fatal("ordinary admin hosts must be unchanged")
	}
}

func TestSparkHostUsesEnforcedStaffPermissions(t *testing.T) {
	a := staffAPI(newFakeStore("enforce"), newFakeIdP().add("pa", "staff.platform_admin").add("sup", "staff.support"))
	a.cfg.Operator.AdminHosts = []string{"models.example.test"}
	a.cfg.Sparks = config.Sparks{TenantID: "owner", ModelAPI: config.SparkEndpoint{State: "published", URL: "https://models.example.test/v1"}}
	r := httptest.NewRequest("GET", "/auth/ws-check", nil)
	for user, want := range map[string]bool{"pa": true, "sup": false} {
		c := &caller{id: testID(user), tenants: []tenantView{{Tenant: db.Tenant{ID: "owner"}}}}
		if got := a.sparkPermit(r, c, "models.example.test"); got != want {
			t.Fatalf("user=%s allowed=%v want=%v", user, got, want)
		}
		if got := a.capabilities(r, c)["host.admin_tools"]; got != want {
			t.Fatalf("capability does not match host check: %s", user)
		}
	}
}
