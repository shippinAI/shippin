package guest

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// Unassign is shippin.infrastructure.server.guest.unassign: POST /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}/unassign.
// The guest keeps running and belongs to no customer afterwards.
var Unassign = mcpl.Def[ByID, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Take a guest away from the customer it is assigned to. The guest itself is not touched.", Access: mcpl.Staff, Perm: "customers.write"},
	Run: func(c *mcpl.Call, in ByID) (hv.Guest, error) {
		_, _, d, err := server.Open(c, in.Server)
		if err != nil {
			return hv.Guest{}, err
		}
		g, err := d.Guest(c.Ctx, in.ID)
		if err != nil {
			return hv.Guest{}, server.Problem(err)
		}
		if _, err := c.DB.Exec(c.Ctx, `DELETE FROM shippin_guest_owners WHERE server_id::text=$1 AND guest=$2`, in.Server, g.ID); err != nil {
			return hv.Guest{}, err
		}
		g.Self = IsSelf(g)
		return g, nil
	},
}
