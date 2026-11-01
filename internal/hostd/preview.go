package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Preview publishes https://<port>--<workspace host label>.<ws base domain> → <ip>:<port>.
// "--" keeps it a single DNS label so the existing *.ws wildcard certificate covers it.
type Preview struct {
	Port   int  `json:"port"`
	Public bool `json:"public"`
}

func (s *Server) validPreview(p int) error {
	e := s.cfg.Edge
	if p < e.PreviewMin || p > e.PreviewMax || p == e.IDEPort {
		return fmt.Errorf("preview port must be %d-%d", e.PreviewMin, e.PreviewMax)
	}
	return nil
}

// validPreviews: at most 20, each port valid and listed once (route ids are per port).
func (s *Server) validPreviews(ps []Preview) error {
	if len(ps) > 20 {
		return fmt.Errorf("at most 20 previews")
	}
	seen := map[int]bool{}
	for _, p := range ps {
		if err := s.validPreview(p.Port); err != nil {
			return err
		}
		if seen[p.Port] {
			return fmt.Errorf("preview port %d listed twice", p.Port)
		}
		seen[p.Port] = true
	}
	return nil
}

// PreviewHost returns <port>--<label>.<base> for the workspace host <label>.<base>.
func PreviewHost(host string, port int) string { return fmt.Sprintf("%d--%s", port, host) }

// SyncPreviews makes the edge's pv-<w>-* routes equal want.
func (s *Server) SyncPreviews(ctx context.Context, w, host, ip string, want []Preview) error {
	if err := s.ValidHost(host); err != nil {
		return err
	}
	if err := s.validPreviews(want); err != nil {
		return err
	}
	if err := s.dropPreviews(ctx, w); err != nil {
		return err
	}
	for _, p := range want {
		rt := s.proxyRoute(fmt.Sprintf("pv-%s-%d", w, p.Port), []string{PreviewHost(host, p.Port)},
			fmt.Sprintf("%s:%d", ip, p.Port), !p.Public, true)
		if _, err := s.caddy(ctx, "PUT", fmt.Sprintf("/config/apps/http/servers/%s/routes/0", s.cfg.Edge.Server), rt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) dropPreviews(ctx context.Context, w string) error {
	raw, err := s.caddy(ctx, "GET", fmt.Sprintf("/config/apps/http/servers/%s/routes", s.cfg.Edge.Server), nil)
	if err != nil {
		return err
	}
	var routes []map[string]any
	_ = json.Unmarshal([]byte(raw), &routes)
	for _, r := range routes {
		if id, _ := r["@id"].(string); strings.HasPrefix(id, "pv-"+w+"-") {
			if _, err := s.caddy(ctx, "DELETE", "/id/"+id, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// Listening returns TCP ports a workspace listens on: reachable (all/eth0 addresses) or local-only.
func (s *Server) listening(w http.ResponseWriter, r *http.Request) {
	at, err := s.placeOf(r.Context(), r.PathValue("t"), r.PathValue("w"))
	if err != nil {
		fail(w, http.StatusConflict, "workspace is not reachable")
		return
	}
	out, err := at.do(r.Context(), "exec", Instance(r.PathValue("w")), "--", "ss", "-Hltn")
	if err != nil {
		fail(w, http.StatusConflict, "workspace is not running")
		return
	}
	seen := map[int]bool{}
	ports := []map[string]any{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 4 {
			continue
		}
		addr := f[3]
		i := strings.LastIndex(addr, ":")
		p, err := strconv.Atoi(addr[i+1:])
		if err != nil || seen[p] || p == s.cfg.Edge.IDEPort || p == 22 {
			continue
		}
		local := strings.HasPrefix(addr, "127.") || strings.HasPrefix(addr, "[::1]")
		if s.validPreview(p) != nil {
			continue
		}
		seen[p] = true
		ports = append(ports, map[string]any{"port": p, "local_only": local})
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i]["port"].(int) < ports[j]["port"].(int) })
	reply(w, map[string]any{"items": ports})
}
