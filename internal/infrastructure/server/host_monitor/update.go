package host_monitor

import (
	"strings"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// UpdateInput turns a server's host monitor on or off.
type UpdateInput struct {
	OnServer
	Enabled bool `json:"enabled"`
	// Image is what the monitor is made from when it is turned on: an image the server has
	// ("local:<fingerprint>") or one a configured image server offers ("images:alpine/3.21"). Any
	// image with a shell does; the smallest is best.
	Image string `json:"image,omitempty"`
}

// Update is shippin.infrastructure.server.host_monitor.update: PUT /v1/operator/infrastructure/servers/{serverId}/host-monitor.
// Turning it on makes one small container on the server, with no network and the host's /proc read-only,
// and starts it. Turning it off removes that container. Nothing else on the server changes.
var Update = mcpl.Def[UpdateInput, State]{
	Spec: mcpl.Spec{Summary: "Turn a server's host monitor on or off. On makes one small container that reads the host's live load, CPU and memory; off removes it.", Access: mcpl.Staff, Perm: "infra.hosts.write", Singleton: true},
	Run: func(c *mcpl.Call, in UpdateInput) (State, error) {
		h, ep, d, err := server.Open(c, in.Server)
		if err != nil {
			return State{}, err
		}
		m, ok := d.(hv.HostMonitored)
		if !ok {
			return State{}, mcpl.Conflict("This kind of server reports its own load and needs no monitor.")
		}
		if !in.Enabled {
			if err := m.DisableHostMonitor(c.Ctx); err != nil {
				return State{}, server.Problem(err)
			}
			return State{Supported: true}, nil
		}
		in.Image = strings.TrimSpace(in.Image)
		if in.Image == "" {
			return State{}, mcpl.Invalid("Name the image to make the monitor from, such as images:alpine/3.21.")
		}
		image := hv.GuestSpec{Image: in.Image}
		if alias, from, protocol, ok := h.ImageSource(c.Ctx, ep.Kind, in.Image); ok {
			image.Image, image.ImageServer, image.ImageProtocol = alias, from, protocol
		}
		st, err := m.EnableHostMonitor(c.Ctx, image)
		return State{Supported: true, HostMonitorState: st}, server.Problem(err)
	},
}
