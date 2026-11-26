package guest

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/mcpl"
)

// Restart is shippin.infrastructure.server.guest.restart: POST /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}/restart.
var Restart = mcpl.Def[PowerInput, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Restart a running guest.", Access: mcpl.Staff, Perm: "infra.guests.power"},
	Run:  func(c *mcpl.Call, in PowerInput) (hv.Guest, error) { return power(c, in, "reboot") },
}
