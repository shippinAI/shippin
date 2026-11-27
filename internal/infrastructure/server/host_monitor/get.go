package host_monitor

import (
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// Get is shippin.infrastructure.server.host_monitor.get: GET /v1/operator/infrastructure/servers/{serverId}/host-monitor.
var Get = mcpl.Def[OnServer, State]{
	Spec: mcpl.Spec{Summary: "See whether a server has the host monitor that reads its live load, CPU and memory, and whether it is running.", Access: mcpl.Staff, Perm: "infra.read", Singleton: true},
	Run: func(c *mcpl.Call, in OnServer) (State, error) {
		m, ok, err := monitored(c, in.Server)
		if err != nil || !ok {
			return State{}, err
		}
		st, err := m.HostMonitor(c.Ctx)
		return State{Supported: true, HostMonitorState: st}, server.Problem(err)
	},
}
