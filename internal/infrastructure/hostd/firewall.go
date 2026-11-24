package hostd

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
)

// Rule is one inbound allow rule for a workspace (enforced by the tenant network ACL).
type Rule struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Source   string `json:"source"`
}

// validRule: tcp/udp, a port that isn't reserved on the shared endpoint, IPv4 source (the
// edge ruleset is "ip" family; anything else would break the whole nft table).
func (s *Server) validRule(r Rule) error {
	if r.Protocol != "tcp" && r.Protocol != "udp" {
		return fmt.Errorf("protocol must be tcp or udp")
	}
	if r.Port < 1 || r.Port > 65535 || s.cfg.reservedPort(r.Port) {
		return fmt.Errorf("port %d is reserved or out of range", r.Port)
	}
	if r.Source != "" {
		if p, err := netip.ParsePrefix(r.Source); err == nil && p.Addr().Is4() {
			return nil
		}
		if a, err := netip.ParseAddr(r.Source); err == nil && a.Is4() {
			return nil
		}
		return fmt.Errorf("source must be an IPv4 address or CIDR")
	}
	return nil
}

type aclRule struct {
	Action          string `json:"action"`
	Protocol        string `json:"protocol"`
	Source          string `json:"source"`
	Destination     string `json:"destination"`
	DestinationPort string `json:"destination_port"`
	Description     string `json:"description"`
}

func (a aclRule) args() []string {
	out := []string{"action=allow", "protocol=" + a.Protocol, "destination=" + a.Destination,
		"destination_port=" + a.DestinationPort, "description=" + a.Description}
	if a.Source != "" {
		out = append(out, "source="+a.Source)
	}
	return out
}

// SyncFirewall makes the ACL's rules tagged ws-<w> equal the desired set (source = the edge,
// since public traffic arrives masqueraded from HubIP; the rule's own source is enforced at the
// edge), then publishes SSH + rule ports on the shared endpoint.
func (s *Server) SyncFirewall(ctx context.Context, t, w, ip string, ssh int, rules []Rule) error {
	pr, tag, hub := "--project="+Project(t), "ws-"+w, s.cfg.Edge.HubIP
	var acl struct {
		Ingress []aclRule `json:"ingress"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/network-acls/%s?project=%s", aclName, Project(t)), &acl); err != nil {
		return err
	}
	want := []aclRule{}
	for _, r := range rules {
		if err := s.validRule(r); err != nil {
			return err
		}
		want = append(want, aclRule{"allow", r.Protocol, hub, ip, fmt.Sprint(r.Port), tag})
	}
	if err := s.SetPorts(ctx, w, s.workspacePorts(ip, ssh, rules)); err != nil {
		return err
	}
	if a := s.cfg.Agent; a.enabled() && a.TermPort > 0 { // edge → agent terminal only, never a public port
		want = append(want, aclRule{"allow", "tcp", hub, ip, fmt.Sprint(a.TermPort), tag})
	}
	have := []aclRule{}
	for _, a := range acl.Ingress {
		if a.Description == tag {
			have = append(have, a)
		}
	}
	for _, a := range have {
		if !slices.Contains(want, a) {
			if _, err := incus(ctx, append([]string{"network", "acl", "rule", "remove", aclName, "ingress", pr}, a.args()...)...); err != nil {
				return err
			}
		}
	}
	for _, a := range want {
		if !slices.Contains(have, a) {
			if _, err := incus(ctx, append([]string{"network", "acl", "rule", "add", aclName, "ingress", pr}, a.args()...)...); err != nil {
				return err
			}
		}
	}
	return nil
}
