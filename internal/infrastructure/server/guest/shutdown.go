package guest

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/mcpl"
)

// Shutdown is shippin.infrastructure.server.guest.shutdown: POST /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}/shutdown.
var Shutdown = mcpl.Def[PowerInput, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Ask a guest's operating system to shut down cleanly.", Access: mcpl.Staff, Perm: "infra.guests.power"},
	Run:  func(c *mcpl.Call, in PowerInput) (hv.Guest, error) { return power(c, in, "shutdown") },
}
