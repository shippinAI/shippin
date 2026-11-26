package guest

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/mcpl"
)

// Stop is shippin.infrastructure.server.guest.stop: POST /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}/stop.
var Stop = mcpl.Def[PowerInput, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Power a guest off at once, without asking its operating system.", Access: mcpl.Staff, Perm: "infra.guests.power"},
	Run:  func(c *mcpl.Call, in PowerInput) (hv.Guest, error) { return power(c, in, "stop") },
}
