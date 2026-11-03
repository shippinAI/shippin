package api

import (
	"net/netip"
	"slices"
	"strings"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
)

func repoMsg(c config.Workspaces) string {
	return "Repository must be a public https URL on " + strings.Join(c.RepoHosts, ", ") + "."
}

// reservedPort: SSH, the SSH allocation range and the edge's own ports.
func reservedPort(p int, c config.Workspaces) bool {
	return p == 22 || (p >= c.PortMin && p <= c.PortMax) || slices.Contains(c.ReservedPorts, p)
}

// checkRules validates firewall rules (published on the shared endpoint). "" = ok.
func checkRules(rules []db.Rule, c config.Workspaces) string {
	if len(rules) > 20 {
		return "At most 20 rules per workspace."
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if r.Protocol != "tcp" && r.Protocol != "udp" {
			return "Protocol must be TCP or UDP."
		}
		if r.Port < 1 || r.Port > 65535 || reservedPort(r.Port, c) {
			return "That port is reserved or out of range."
		}
		if r.Source != "" && !ipv4OrCIDR(r.Source) {
			return "Source must be an IPv4 address or CIDR."
		}
		key := r.Protocol + "/" + itoa(r.Port)
		if seen[key] {
			return "Each port can only have one rule."
		}
		seen[key] = true
	}
	return ""
}

func ipv4OrCIDR(s string) bool {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Addr().Is4()
	}
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4()
}

// checkDomains lower-cases and validates custom domains in place; platform names are refused.
func checkDomains(ds []string, c config.Workspaces) string {
	if len(ds) > 10 {
		return "At most 10 custom domains per workspace."
	}
	for i, d := range ds {
		d = strings.ToLower(strings.TrimSpace(d))
		ds[i] = d
		if hostd.ValidDomain(d) != nil {
			return d + " isn't a valid custom domain."
		}
		for _, r := range append([]string{c.BaseDomain}, c.ReservedDomains...) {
			if d == r || strings.HasSuffix(d, "."+r) {
				return d + " is a platform domain and can't be attached."
			}
		}
		if slices.Contains(ds[:i], d) {
			return d + " is listed twice."
		}
	}
	return ""
}

// checkPreviews: at most 20, unique, inside the preview range, not the IDE port.
func checkPreviews(ps []db.Preview, c config.Workspaces) string {
	if len(ps) > 20 {
		return "At most 20 previews per workspace."
	}
	seen := map[int]bool{}
	for _, p := range ps {
		if p.Port < c.PreviewMin || p.Port > c.PreviewMax || p.Port == c.IDEPort || seen[p.Port] {
			return "Preview ports must be unique and between " + itoa(c.PreviewMin) + " and " + itoa(c.PreviewMax) + "."
		}
		seen[p.Port] = true
	}
	return ""
}
