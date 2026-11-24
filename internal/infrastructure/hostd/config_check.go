package hostd

import (
	"fmt"
	"net/netip"
	"strings"
)

// Check fails closed on settings whose zero value would widen what hostd allows.
func (c *Config) Check() error {
	e := c.Edge
	switch {
	case c.PortMin < 1024 || c.PortMax < c.PortMin || c.PortMax > 65535:
		return fmt.Errorf("port_min/port_max must be a range within 1024-65535")
	case e.PreviewMin < 1024 || e.PreviewMax < e.PreviewMin || e.PreviewMax > 65535:
		return fmt.Errorf("edge.preview_min/preview_max must be a range within 1024-65535")
	case e.ImagePrefix == "" || !strings.HasSuffix(e.ImagePrefix, "/"):
		return fmt.Errorf("edge.image_prefix must be set and end with /")
	case e.BaseDomain == "" || e.Server == "" || e.Instance == "":
		return fmt.Errorf("edge.ws_base_domain, caddy_server and instance are required")
	case !strings.HasPrefix(e.CaddyAdmin, "http://localhost:") && !strings.HasPrefix(e.CaddyAdmin, "http://127.0.0.1:"):
		return fmt.Errorf("edge.caddy_admin must be a localhost URL")
	case e.WSCookie == "" || strings.ContainsAny(e.WSCookie, "=; \t"):
		return fmt.Errorf("edge.ws_cookie must be a cookie name")
	case len(e.RepoHosts) == 0:
		return fmt.Errorf("edge.repo_hosts must list at least one git host")
	case len(e.ReservedDomains) == 0:
		return fmt.Errorf("edge.reserved_domains must list the platform domain")
	}
	for _, h := range e.RepoHosts {
		if ValidDomain(h) != nil {
			return fmt.Errorf("edge.repo_hosts: %q is not a hostname", h)
		}
	}
	if c.VMFirst && (c.TenantVMImage == "" || !strings.HasPrefix(c.TenantVMImage, e.ImagePrefix)) {
		return fmt.Errorf("vm_first needs tenant_vm_image under edge.image_prefix")
	}
	for name, p := range c.Plans {
		if v := p.VM; v != nil && (v.CPU < 1 || v.Memory == "" || v.RootGiB < 8 || v.DataGiB < 10) {
			return fmt.Errorf("plans.%s.vm: cpu, memory, root_gib>=8 and data_gib>=10 are required", name)
		}
		root, data := p.RootGiB, 0
		if p.VM != nil {
			root, data = p.VM.RootGiB, p.VM.DataGiB
		}
		if p.QuotaGiB < root+data {
			return fmt.Errorf("plans.%s.quota_gib must cover root_gib + data_gib on the org dataset", name)
		}
		if p.Cowork {
			if p.VM == nil || p.VM.DataGiB < CoworkDiskGiB {
				return fmt.Errorf("plans.%s.cowork needs a data volume of at least %d GiB (that disk is not the VM root)", name, CoworkDiskGiB)
			}
			if gib, ok := memGiB(p.VM.Memory); !ok || gib < 8 {
				return fmt.Errorf("plans.%s.cowork needs at least 8 GiB of VM RAM", name)
			}
		}
	}
	if _, err := netip.ParsePrefix(c.TenantCIDR); err != nil {
		return fmt.Errorf("tenant_cidr: %v", err)
	}
	if err := c.Connect.check(); err != nil {
		return err
	}
	if c.Connect.enabled() && !c.AI.enabled() {
		return fmt.Errorf("connect needs the ai section (user, uid, home)")
	}
	if err := c.Git.check(); err != nil {
		return err
	}
	if err := c.Agent.check(); err != nil {
		return err
	}
	if err := c.Enroll.Check(); err != nil {
		return err
	}
	if err := c.Services.check(e.ImagePrefix); err != nil {
		return err
	}
	return c.AI.check(e.ImagePrefix)
}

// reservedPort: the edge's own ports, SSH, and the SSH allocation range are never rule ports.
func (c *Config) reservedPort(p int) bool {
	if p == 22 || (p >= c.PortMin && p <= c.PortMax) {
		return true
	}
	for _, r := range c.Edge.ReservedPorts {
		if p == r {
			return true
		}
	}
	return false
}

// reservedDomain: a platform hostname (equal to or under a reserved domain, or the ws base).
func (c *Config) reservedDomain(d string) bool {
	for _, r := range append([]string{c.Edge.BaseDomain}, c.Edge.ReservedDomains...) {
		if d == r || strings.HasSuffix(d, "."+r) {
			return true
		}
	}
	return false
}
