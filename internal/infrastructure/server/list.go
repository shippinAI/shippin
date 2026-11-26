package server

import (
	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/mcpl"
)

// List is shippin.infrastructure.server.list: GET /v1/operator/infrastructure/servers. A server's
// secret is never part of the answer; has_secret says one is stored.
var List = mcpl.Def[mcpl.None, mcpl.Items[servers.Endpoint]]{
	Spec: mcpl.Spec{Summary: "List the servers this installation has added, with each one's kind and address.", Access: mcpl.Staff, Perm: "infra.read"},
	Run: func(c *mcpl.Call, _ mcpl.None) (mcpl.Items[servers.Endpoint], error) {
		out := mcpl.Items[servers.Endpoint]{Items: []servers.Endpoint{}}
		h, err := Hosts(c)
		if err != nil {
			return out, err
		}
		items, err := h.Servers(c.Ctx)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, items...)
		return out, nil
	},
}
