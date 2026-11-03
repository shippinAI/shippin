package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"sync"
)

const aclName = "ws-ingress"

var netMu sync.Mutex

type netInfo struct {
	Config map[string]string `json:"config"`
}

// tenantNetwork ensures n-<t> exists on the transit uplink with a /24 from TenantCIDR.
// Returns created=true when this call made it (so a failed provision can roll it back).
// A network still on the legacy LAN uplink is migrated.
func (s *Server) tenantNetwork(ctx context.Context, t string) (created bool, err error) {
	pr := "--project=" + Project(t)
	var n netInfo
	if query(ctx, fmt.Sprintf("/1.0/networks/%s?project=%s", Network(t), Project(t)), &n) == nil {
		if n.Config["network"] == s.cfg.Transit {
			return false, nil
		}
		return false, s.migrateNetwork(ctx, t)
	}
	return true, s.createTenantNet(ctx, t, pr)
}

func (s *Server) createTenantNet(ctx context.Context, t, pr string) error {
	netMu.Lock()
	defer netMu.Unlock()
	sub, err := s.freeSubnet(ctx)
	if err != nil {
		return err
	}
	gw := sub.Addr().Next()
	_, err = incus(ctx, "network", "create", Network(t), pr, "--type=ovn", "network="+s.cfg.Transit,
		fmt.Sprintf("ipv4.address=%s/24", gw), "ipv4.nat=true", "ipv6.address=none", "user.shippin.tenant="+t)
	return err
}

// freeSubnet picks the first /24 inside TenantCIDR not used by any OVN network nor routed to a
// tenant VM as its inner subnet. Callers hold netMu.
func (s *Server) freeSubnet(ctx context.Context) (netip.Prefix, error) {
	cidr, err := netip.ParsePrefix(s.cfg.TenantCIDR)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("bad tenant_cidr")
	}
	var nets []netInfo
	out, err := incus(ctx, "query", "/1.0/networks?all-projects=true&recursion=1")
	if err != nil {
		return netip.Prefix{}, err
	}
	if err := json.Unmarshal([]byte(out), &nets); err != nil {
		return netip.Prefix{}, err
	}
	used := map[netip.Prefix]bool{}
	for _, n := range nets {
		if p, err := netip.ParsePrefix(n.Config["ipv4.address"]); err == nil {
			used[p.Masked()] = true
		}
		if p, err := netip.ParsePrefix(n.Config[innerKey]); err == nil {
			used[p.Masked()] = true
		}
	}
	for a := cidr.Masked().Addr(); cidr.Contains(a); {
		p := netip.PrefixFrom(a, 24)
		if !used[p] {
			return p, nil
		}
		b := a.As4()
		if b[2] == 255 {
			b[1], b[2] = b[1]+1, 0
		} else {
			b[2]++
		}
		a = netip.AddrFrom4(b)
	}
	return netip.Prefix{}, fmt.Errorf("tenant address space %s is full", s.cfg.TenantCIDR)
}

// connectHub peers the tenant network with the hub (both ways) and sets the ingress ACL:
// only the edge (HubIP) may reach workspaces, everything else is dropped.
func (s *Server) connectHub(ctx context.Context, t string) error {
	e, pr := s.cfg.Edge, "--project="+Project(t)
	if err := ignoreExists(incus(ctx, "network", "peer", "create", e.HubNetwork, Project(t),
		Project(t)+"/"+Network(t), "--project="+s.cfg.Platform)); err != nil {
		return err
	}
	if err := ignoreExists(incus(ctx, "network", "peer", "create", Network(t), "hub",
		s.cfg.Platform+"/"+e.HubNetwork, pr)); err != nil {
		return err
	}
	if !exists(ctx, "network", "acl", "show", aclName, pr) {
		if _, err := incus(ctx, "network", "acl", "create", aclName, pr); err != nil {
			return err
		}
	}
	if err := s.baseACL(ctx, t); err != nil {
		return err
	}
	_, err := incus(ctx, "network", "set", Network(t), pr, "security.acls="+aclName,
		"security.acls.default.ingress.action=drop", "security.acls.default.egress.action=allow")
	return err
}

// baseACL rewrites the ACL: fixed base rules plus per-workspace rules (description ws-*),
// whose source is forced to the edge (public ports arrive masqueraded from HubIP).
func (s *Server) baseACL(ctx context.Context, t string) error {
	e := s.cfg.Edge
	var acl struct {
		Ingress []aclRule `json:"ingress"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/network-acls/%s?project=%s", aclName, Project(t)), &acl); err != nil {
		return err
	}
	rules := []map[string]string{
		{"action": "allow", "protocol": "tcp", "destination_port": fmt.Sprint(e.IDEPort), "source": e.HubIP, "state": "enabled"},
		{"action": "allow", "protocol": "tcp", "destination_port": "22", "source": e.HubIP, "state": "enabled"},
		{"action": "allow", "source": "@internal", "state": "enabled"},
	}
	if e.PreviewPorts != "" {
		rules = append(rules, map[string]string{"action": "allow", "protocol": "tcp", "destination_port": e.PreviewPorts,
			"source": e.HubIP, "description": "previews", "state": "enabled"})
	}
	rules = append(rules, []map[string]string{
		{"action": "allow", "protocol": "icmp4", "state": "enabled"},
	}...)
	for _, a := range acl.Ingress {
		if strings.HasPrefix(a.Description, "ws-") {
			rules = append(rules, map[string]string{"action": "allow", "protocol": a.Protocol, "source": e.HubIP,
				"destination": a.Destination, "destination_port": a.DestinationPort, "description": a.Description, "state": "enabled"})
		}
	}
	body, _ := json.Marshal(map[string]any{"ingress": rules, "egress": []any{}})
	_, err := incus(ctx, "query", "-X", "PUT", "--data", string(body), fmt.Sprintf("/1.0/network-acls/%s?project=%s", aclName, Project(t)))
	return err
}
