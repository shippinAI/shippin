package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
)

// InnerProject is the legacy fixed inner project (set by shippin-tvm-init); M1 workspaces live in
// one inner project per Shippin project instead (ProjectInner). InnerPool is fixed in every VM.
const (
	InnerProject = "ws"
	InnerPool    = "default"
	innerBridge  = "incusbr0" // the routed inner bridge shippin-tvm-init creates
	maxGuestOut  = 8 << 20
)

func TenantVM(t string) string      { return "tvm-" + t }
func TenantVMData(t string) string  { return "tvd-" + t }
func BoundaryVM(b string) string    { return "b-" + b }
func ProjectInner(pj string) string { return "pj-" + pj }

// place says where a workspace's Incus calls go: the host tenant project (legacy direct) or the
// Incus daemon inside the tenant VM, reached only through the VM agent (incus exec, vsock).
type place struct {
	t     string
	vm    bool
	inner netip.Prefix // tenant VM's routed inner /24 (vm only)
	pj    string       // Shippin project UUID; empty = legacy inner project "ws"
}

// in returns the same placement scoped to Shippin project pj (VM placement only).
func (p place) in(pj string) place {
	p.pj = pj
	return p
}

func (p place) project() string {
	if p.vm && p.pj != "" {
		return ProjectInner(p.pj)
	}
	if p.vm {
		return InnerProject
	}
	return Project(p.t)
}

func (p place) pool() string {
	if p.vm {
		return InnerPool
	}
	return Pool(p.t)
}

// inc runs `incus --project=<p> args...` on the host or, for a VM tenant, inside its VM.
// Guest output is untrusted: it is size-bounded here and only ever parsed into typed values.
func (p place) inc(ctx context.Context, stdin string, args ...string) (string, error) {
	out, err := runner(ctx, stdin, "incus", p.argv(args)...)
	if p.vm && len(out) > maxGuestOut {
		return "", fmt.Errorf("tenant VM output too large")
	}
	return out, err
}

// argv is the host incus argv: the project flag first (before any "--" of exec), wrapped in an
// agent exec into the tenant VM for VM placement.
func (p place) argv(args []string) []string {
	a := args
	if len(args) == 0 || args[0] != "query" { // query takes the project in its path only
		a = append([]string{"--project=" + p.project()}, args...)
	}
	if !p.vm {
		return a
	}
	return append([]string{"exec", TenantVM(p.t), "--project=" + Project(p.t), "--", "incus"}, a...)
}

func (p place) do(ctx context.Context, args ...string) (string, error) {
	return p.inc(ctx, "", args...)
}

func (p place) has(ctx context.Context, args ...string) bool {
	_, err := p.do(ctx, args...)
	return err == nil
}

// query GETs an API path (without ?project) in this place's project and decodes it.
func (p place) query(ctx context.Context, path string, out any) error {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	s, err := p.do(ctx, "query", path+sep+"project="+p.project())
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), out)
}

// queryAll GETs an API path across every project in this place. Incus 6.0 rejects
// all-projects together with project= (400), and the CLI rejects --project next to that
// URL, so this must not go through query.
func (p place) queryAll(ctx context.Context, path string, out any) error {
	if strings.Contains(path, "all-projects=") || strings.Contains(path, "project=") {
		return fmt.Errorf("inner query path must not set project")
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	s, err := p.do(ctx, "query", path+sep+"all-projects=true")
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), out)
}

// checkIP: an address reported by a tenant VM must be inside that VM's inner subnet, so a
// compromised guest can never steer an edge route at another tenant's address.
func (p place) checkIP(ip string) error {
	if !p.vm {
		return nil
	}
	a, err := netip.ParseAddr(ip)
	if err != nil || !p.inner.Contains(a) {
		return fmt.Errorf("tenant VM reported an address outside its subnet")
	}
	return nil
}

// placeOf resolves a workspace: an instance in the host project is legacy direct; otherwise it
// lives in the tenant VM when the tenant is VM-backed.
func (s *Server) placeOf(ctx context.Context, t, w string) (place, error) {
	if exists(ctx, "info", Instance(w), "--project="+Project(t)) {
		return place{t: t}, nil
	}
	at, err := s.newPlace(ctx, t)
	if err != nil || !at.vm {
		return at, err
	}
	return at.in(s.innerProjectOf(ctx, at, w)), nil
}

// newPlace is where a new workspace of t goes: the tenant VM if the tenant is VM-backed.
func (s *Server) newPlace(ctx context.Context, t string) (place, error) {
	m, err := s.tenantMode(ctx, t)
	if err != nil || m.Mode != modeVM {
		return place{t: t}, err
	}
	if m.Inner == "" {
		return place{}, fmt.Errorf("tenant VM is not provisioned")
	}
	pre, err := netip.ParsePrefix(m.Inner)
	if err != nil {
		return place{}, fmt.Errorf("tenant VM subnet invalid")
	}
	return place{t: t, vm: true, inner: pre}, nil
}
