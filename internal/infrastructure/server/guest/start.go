package guest

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/mcpl"
)

// Start is shippin.infrastructure.server.guest.start: POST /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}/start.
var Start = mcpl.Def[PowerInput, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Start a stopped guest.", Access: mcpl.Staff, Perm: "infra.guests.power"},
	Run:  func(c *mcpl.Call, in PowerInput) (hv.Guest, error) { return power(c, in, "start") },
}
