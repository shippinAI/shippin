package guest

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// List is shippin.infrastructure.server.guest.list: GET /v1/operator/infrastructure/servers/{serverId}/guests.
// The guest this panel runs on is marked self. A guest the panel just changed shows the state that
// change left it in while the hypervisor's own inventory catches up.
var List = mcpl.Def[OnServer, mcpl.Items[hv.Guest]]{
	Spec: mcpl.Spec{Summary: "List the virtual machines and containers on a server, with each one's state.", Access: mcpl.Staff, Perm: "infra.read"},
	Run: func(c *mcpl.Call, in OnServer) (mcpl.Items[hv.Guest], error) {
		out := mcpl.Items[hv.Guest]{Items: []hv.Guest{}}
		h, _, d, err := server.Open(c, in.Server)
		if err != nil {
			return out, err
		}
		items, err := d.Guests(c.Ctx)
		if err != nil {
			return out, server.Problem(err)
		}
		recent := h.RecentOutcomes(c.Ctx, in.Server)
		for i := range items {
			if status, ok := recent[items[i].ID]; ok {
				items[i].Status = status
			}
		}
		out.Items = append(out.Items, MarkSelf(items)...)
		return out, nil
	},
}
