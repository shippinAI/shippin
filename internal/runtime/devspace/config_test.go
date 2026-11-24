package devspace

import (
	"shippin.cloud/console/internal/platform/config"
)

// testRules has the shape of the workspaces section in config/shippin.example.json.
func testRules() config.Workspaces {
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
