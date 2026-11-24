package hostd

import (
	"context"
	"fmt"
	"net/netip"
)

const (
	modeVM     = "vm"
	modeKey    = "user.shippin.mode"
	planKey    = "user.shippin.plan"
	innerKey   = "user.shippin.inner_subnet"
	vmAddrHost = 254 // tenant VM address inside the tenant /24 (OVN hands out dynamic ones from .2 up)
)

type tenantMode struct {
	Mode  string // "vm" or "" (legacy direct)
	Inner string // inner /24 routed to the tenant VM
	Net   string // tenant network gateway cidr (e.g. 10.128.3.1/24)
	Plan  string // user.shippin.plan, when provision recorded it
}

func (s *Server) tenantMode(ctx context.Context, t string) (tenantMode, error) {
	var pj, n struct {
		Config map[string]string `json:"config"`
	}
	if err := query(ctx, "/1.0/projects/"+Project(t), &pj); err != nil {
		return tenantMode{}, fmt.Errorf("tenant project not found")
	}
	_ = query(ctx, fmt.Sprintf("/1.0/networks/%s?project=%s", Network(t), Project(t)), &n)
	return tenantMode{pj.Config[modeKey], n.Config[innerKey], n.Config["ipv4.address"], pj.Config[planKey]}, nil
}

// vmAddr is the tenant VM's static address in the tenant network.
func vmAddr(netCIDR string) (string, error) {
	p, err := netip.ParsePrefix(netCIDR)
	if err != nil || p.Bits() != 24 || !p.Addr().Is4() {
		return "", fmt.Errorf("tenant network has no /24")
	}
	b := p.Masked().Addr().As4()
	b[3] = vmAddrHost
	return netip.AddrFrom4(b).String(), nil
}

// innerGateway is the inner bridge address (.1) of an inner /24.
func innerGateway(inner string) (string, error) {
	p, err := netip.ParsePrefix(inner)
	if err != nil || p.Bits() != 24 || !p.Addr().Is4() {
		return "", fmt.Errorf("inner subnet must be a /24")
	}
	return p.Masked().Addr().Next().String() + "/24", nil
}

// ensureInnerSubnet allocates (once) a free /24 from tenant_cidr for the tenant VM's containers
// and records it on the tenant network, under netMu so two tenants never get the same one.
func (s *Server) ensureInnerSubnet(ctx context.Context, t string) (string, error) {
	netMu.Lock()
	defer netMu.Unlock()
	m, err := s.tenantMode(ctx, t)
	if err != nil || m.Inner != "" {
		return m.Inner, err
	}
	sub, err := s.freeSubnet(ctx)
	if err != nil {
		return "", err
	}
	_, err = incus(ctx, "network", "set", Network(t), "--project="+Project(t), innerKey+"="+sub.String())
	return sub.String(), err
}
