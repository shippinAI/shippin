package hostd

func testServer() *Server {
	return &Server{cfg: &Config{
		PortMin: 22000, PortMax: 22999, TenantCIDR: "10.128.0.0/9", Image: "shippin/dev-debian13", Platform: "platform",
		Plans: map[string]Plan{"founder": {HomeGiB: 50}},
		Edge: Edge{Instance: "edge", BaseDomain: "ws.example.test", IDEPort: 3000, Server: "srv0", Console: "192.0.2.5:8080",
			PreviewMin: 1024, PreviewMax: 65535, ImagePrefix: "shippin/", CaddyAdmin: "http://localhost:2019", WSCookie: "shippin_ws",
			RepoHosts: []string{"github.com", "gitlab.com"}, ReservedPorts: []int{22, 80, 443}, ReservedDomains: []string{"example.test"}},
	}}
}
