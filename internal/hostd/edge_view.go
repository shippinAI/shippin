package hostd

import (
	"encoding/json"
	"net/http"
	"sort"
)

// EdgeRoute is a read-only summary of one Caddy route on the edge.
type EdgeRoute struct {
	ID        string   `json:"id"`
	Server    string   `json:"server"`
	Hosts     []string `json:"hosts"`
	Upstreams []string `json:"upstreams"`
	Auth      bool     `json:"forward_auth"`
}

func (s *Server) edgeRoutes(w http.ResponseWriter, r *http.Request) {
	raw, err := s.caddy(r.Context(), "GET", "/config/apps/http/servers", nil)
	if err != nil {
		fail(w, http.StatusBadGateway, "edge admin API unavailable")
		return
	}
	var servers map[string]struct {
		Listen []string         `json:"listen"`
		Routes []map[string]any `json:"routes"`
	}
	if json.Unmarshal([]byte(raw), &servers) != nil {
		fail(w, http.StatusBadGateway, "unexpected edge config")
		return
	}
	out := []EdgeRoute{}
	for name, srv := range servers {
		for _, rt := range srv.Routes {
			e := EdgeRoute{Server: name, Hosts: []string{}, Upstreams: []string{}}
			e.ID, _ = rt["@id"].(string)
			walk(rt, &e)
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return first(out[i].Hosts) < first(out[j].Hosts) })
	reply(w, map[string]any{"items": out})
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// walk collects host matchers and reverse_proxy upstreams from a route tree.
func walk(v any, e *EdgeRoute) {
	switch x := v.(type) {
	case map[string]any:
		if hs, ok := x["host"].([]any); ok {
			for _, h := range hs {
				if s, ok := h.(string); ok {
					e.Hosts = append(e.Hosts, s)
				}
			}
		}
		if x["handler"] == "reverse_proxy" {
			if _, ok := x["rewrite"]; ok {
				e.Auth = true
			} else if ups, ok := x["upstreams"].([]any); ok {
				for _, u := range ups {
					if m, ok := u.(map[string]any); ok {
						if d, ok := m["dial"].(string); ok {
							e.Upstreams = append(e.Upstreams, d)
						}
					}
				}
			}
		}
		for _, c := range x {
			walk(c, e)
		}
	case []any:
		for _, c := range x {
			walk(c, e)
		}
	}
}

func (s *Server) edgePorts(w http.ResponseWriter, _ *http.Request) {
	ps, err := s.loadPorts()
	if err != nil {
		fail(w, http.StatusInternalServerError, "port state unreadable")
		return
	}
	reply(w, map[string]any{"public_ip": s.cfg.Edge.PublicIP, "min": s.cfg.PortMin, "max": s.cfg.PortMax, "items": ps})
}

// tenantNetworks lists every tenant OVN network with its subnet (read-only).
func (s *Server) tenantNetworks(w http.ResponseWriter, r *http.Request) {
	var nets []struct {
		Name    string            `json:"name"`
		Project string            `json:"project"`
		Config  map[string]string `json:"config"`
	}
	if err := query(r.Context(), "/1.0/networks?all-projects=true&recursion=1", &nets); err != nil {
		fail(w, http.StatusBadGateway, "incus unavailable")
		return
	}
	out := map[string]map[string]string{}
	for _, n := range nets {
		if n.Project != s.cfg.Platform && n.Project != "default" && n.Config["ipv4.address"] != "" {
			out[n.Project] = map[string]string{"network": n.Name, "subnet": n.Config["ipv4.address"], "router": n.Config["volatile.network.ipv4.address"]}
		}
	}
	reply(w, out)
}
