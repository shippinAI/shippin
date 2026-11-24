package hostd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Only a complete readable forward record may establish that an earlier cleanup
// removed this port. Unknown/malformed provider state keeps cleanup failed.
func (s *Server) forwardPortAbsent(ctx context.Context, proto, port string) bool {
	var record struct {
		Ports *[]struct {
			Protocol   string `json:"protocol"`
			ListenPort string `json:"listen_port"`
		} `json:"ports"`
	}
	path := fmt.Sprintf("/1.0/networks/%s/forwards/%s?project=%s", url.PathEscape(s.cfg.Edge.PlatformNetwork), url.PathEscape(s.cfg.Edge.PublicIP), url.QueryEscape(s.cfg.Platform))
	if query(ctx, path, &record) != nil || record.Ports == nil {
		return false
	}
	wanted, e := strconv.Atoi(port)
	if e != nil {
		return false
	}
	present := false
	for _, p := range *record.Ports {
		if p.Protocol != "tcp" && p.Protocol != "udp" {
			return false
		}
		for _, part := range strings.Split(p.ListenPort, ",") {
			bounds := strings.Split(strings.TrimSpace(part), "-")
			if len(bounds) > 2 {
				return false
			}
			lo, e := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if e != nil || lo < 1 || lo > 65535 {
				return false
			}
			hi := lo
			if len(bounds) == 2 {
				hi, e = strconv.Atoi(strings.TrimSpace(bounds[1]))
				if e != nil || hi < lo || hi > 65535 {
					return false
				}
			}
			present = present || (p.Protocol == proto && wanted >= lo && wanted <= hi)
		}
	}
	return !present
}
