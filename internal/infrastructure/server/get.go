package server

import (
	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/mcpl"
)

// Get is shippin.infrastructure.server.get: GET /v1/operator/infrastructure/servers/{serverId}.
var Get = mcpl.Def[ByID, servers.Endpoint]{
	Spec: mcpl.Spec{Summary: "Read one server: its kind, address and whether a credential is stored.", Access: mcpl.Staff, Perm: "infra.read"},
	Run: func(c *mcpl.Call, in ByID) (servers.Endpoint, error) {
		h, err := Hosts(c)
		if err != nil {
			return servers.Endpoint{}, err
		}
		ep, err := h.Server(c.Ctx, in.ID)
		return ep, Problem(err)
	},
}
