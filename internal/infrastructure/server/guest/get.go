package guest

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// Get is shippin.infrastructure.server.guest.get: GET /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}.
var Get = mcpl.Def[ByID, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Read one guest on a server: its kind, state and what it is using.", Access: mcpl.Staff, Perm: "infra.read"},
	Run: func(c *mcpl.Call, in ByID) (hv.Guest, error) {
		_, _, d, err := server.Open(c, in.Server)
		if err != nil {
			return hv.Guest{}, err
		}
		g, err := d.Guest(c.Ctx, in.ID)
		if err != nil {
			return hv.Guest{}, server.Problem(err)
		}
		g.Self = IsSelf(g)
		owned, err := owners(c, in.Server)
		if err != nil {
			return hv.Guest{}, err
		}
		if o, ok := owned[g.ID]; ok {
			g.Owner = &o
		}
		return g, nil
	},
}
