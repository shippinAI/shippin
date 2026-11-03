package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Port is one port on the shared public endpoint, DNATed by the edge to a workspace.
type Port struct {
	Proto  string `json:"proto"`
	Port   int    `json:"port"`
	Dest   string `json:"dest"`
	DPort  int    `json:"dport"`
	Source string `json:"source"`
	Owner  string `json:"owner"`
}

var portsMu sync.Mutex

func (s *Server) portsFile() string { return filepath.Join(s.cfg.StateDir, "ports.json") }

func (s *Server) loadPorts() ([]Port, error) {
	raw, err := os.ReadFile(s.portsFile())
	if os.IsNotExist(err) {
		return []Port{}, nil
	}
	var ps []Port
	if err == nil {
		err = json.Unmarshal(raw, &ps)
	}
	return ps, err
}

func (s *Server) savePorts(ps []Port) error {
	_ = os.MkdirAll(s.cfg.StateDir, 0o700)
	b, _ := json.MarshalIndent(ps, "", " ")
	tmp := s.portsFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.portsFile())
}

// SetPorts replaces every shared-endpoint port owned by owner with want. A port held by
// another owner is a conflict. The edge ruleset and the platform forward are then synced.
func (s *Server) SetPorts(ctx context.Context, owner string, want []Port) error {
	portsMu.Lock()
	defer portsMu.Unlock()
	all, err := s.loadPorts()
	if err != nil {
		return err
	}
	keep := []Port{}
	for _, p := range all {
		if p.Owner != owner {
			keep = append(keep, p)
		}
	}
	for _, w := range want {
		for _, p := range keep {
			if p.Proto == w.Proto && p.Port == w.Port {
				return fmt.Errorf("port %d/%s is already in use on the shared endpoint", w.Port, w.Proto)
			}
		}
		w.Owner = owner
		keep = append(keep, w)
	}
	sort.Slice(keep, func(i, j int) bool { return keep[i].Port < keep[j].Port })
	if err := s.applyPorts(ctx, all, keep); err != nil {
		return err
	}
	return s.savePorts(keep)
}

// applyPorts renders the edge nft table and adds/removes ports on the platform forward.
func (s *Server) applyPorts(ctx context.Context, before, after []Port) error {
	e, pp := s.cfg.Edge, "--project="+s.cfg.Platform
	key := func(p Port) string { return fmt.Sprintf("%s/%d", p.Proto, p.Port) }
	had, has := map[string]bool{}, map[string]bool{}
	for _, p := range before {
		had[key(p)] = true
	}
	for _, p := range after {
		has[key(p)] = true
		if !had[key(p)] {
			if err := ignoreExists(incus(ctx, "network", "forward", "port", "add", e.PlatformNetwork, e.PublicIP,
				p.Proto, fmt.Sprint(p.Port), e.IP, pp)); err != nil && !strings.Contains(err.Error(), "already") {
				return err
			}
		}
	}
	for k := range had {
		if !has[k] {
			proto, port, _ := strings.Cut(k, "/")
			if _, err := incus(ctx, "network", "forward", "port", "remove", e.PlatformNetwork, e.PublicIP, proto, port, pp); err != nil {
				if !s.forwardPortAbsent(ctx, proto, port) {
					return err
				}
			}
		}
	}
	f, err := os.CreateTemp(s.cfg.StateDir, "ports-*.nft")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString(s.renderNft(after))
	_ = f.Close()
	if _, err := incus(ctx, "file", "push", f.Name(), e.Instance+"/etc/shippin/ports.nft", pp, "--mode=0600"); err != nil {
		return err
	}
	_, err = incus(ctx, "exec", e.Instance, pp, "--", "nft", "-f", "/etc/shippin/ports.nft")
	return err
}

func (s *Server) renderNft(ps []Port) string {
	e := s.cfg.Edge
	var b strings.Builder
	b.WriteString("table ip shippin\ndelete table ip shippin\ntable ip shippin {\n")
	b.WriteString(" chain pre {\n  type nat hook prerouting priority dstnat; policy accept;\n")
	for _, p := range ps {
		src := ""
		if p.Source != "" && p.Source != "0.0.0.0/0" {
			src = "ip saddr " + p.Source + " "
		}
		fmt.Fprintf(&b, "  iifname != %q %s%s dport %d dnat to %s:%d\n", e.HubIface, src, p.Proto, p.Port, p.Dest, p.DPort)
	}
	b.WriteString(" }\n chain post {\n  type nat hook postrouting priority srcnat; policy accept;\n")
	fmt.Fprintf(&b, "  oifname %q ip daddr %s masquerade\n }\n}\n", e.HubIface, s.cfg.TenantCIDR)
	return b.String()
}

// workspacePorts: SSH plus each firewall rule, all on the shared endpoint.
func (s *Server) workspacePorts(ip string, ssh int, rules []Rule) []Port {
	out := []Port{}
	if ssh > 0 {
		out = append(out, Port{Proto: "tcp", Port: ssh, Dest: ip, DPort: 22})
	}
	for _, r := range rules {
		p := Port{Proto: r.Protocol, Port: r.Port, Dest: ip, DPort: r.Port, Source: r.Source}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
