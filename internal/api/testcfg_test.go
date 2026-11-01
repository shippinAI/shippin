package api

import "shippin.cloud/console/internal/config"

// testWS is a representative workspaces config (same shape as config/shippin.example.json).
func testWS() config.Workspaces {
	return config.Workspaces{
		PortMin: 22000, PortMax: 22999, BaseDomain: "ws.example.test", IDEPort: 3000,
		PreviewMin: 1024, PreviewMax: 65535, ImagePrefix: "shippin/",
		Guards: config.Guards{
			RepoHosts: []string{"github.com", "gitlab.com"}, ReservedPorts: []int{22, 80, 443},
			ReservedDomains: []string{"example.test"},
			TemplateLimits:  config.TemplateLimits{MaxCPU: 32, MaxMemoryGiB: 128, MinDiskGiB: 5, MaxDiskGiB: 1024, MaxPorts: 10, MaxStartup: 16384},
		},
	}
}

// testCfg mirrors the role and access sections of config/shippin.example.json.
func testCfg() *config.Config {
	return &config.Config{
		Workspaces: testWS(),
		Operator:   config.Operator{Role: "op_superadmin"},
		Roles: config.Roles{
			IAM:        []config.RoleMap{{From: "IAM_OWNER", To: "op_superadmin"}, {From: "IAM_ORG_MANAGER", To: "op_admin"}},
			IAMDefault: "op_support",
			Org:        []config.RoleMap{{From: "ORG_OWNER", To: "org_owner"}, {From: "ORG_USER_MANAGER", To: "org_admin"}},
			Member:     "org_member", Owner: "org_owner", Managers: []string{"org_owner", "org_admin"},
		},
		Access: config.Access{
			Durations: []string{"1h", "4h", "24h"}, ReasonMin: 10, ReasonMax: 500, SweepSeconds: 30, LogWindow: "10m",
			BreakGlass: config.BreakGlass{Duration: "1h", ReauthMinutes: 10, JustificationMin: 20, NoticeHours: 168,
				Categories: []config.Category{{ID: "security_incident", Label: "Security incident"},
					{ID: "legal_hold", Label: "Legal hold", WithholdNotice: true, NoticeDelay: "720h"}}},
		},
	}
}
