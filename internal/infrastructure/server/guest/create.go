package guest

import (
	"context"
	"slices"
	"strings"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// Kind is what a guest is: a virtual machine or a container.
type Kind string

func (Kind) Values() []string { return []string{hv.KindVM, hv.KindCT} }

// CreateInput is a guest to make. Storage, network and disk apply only where the server's kind takes
// them; a size left at zero keeps the hypervisor's default.
type CreateInput struct {
	OnServer
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
	// Image is an image the server has, or one this installation names as "remote:alias".
	Image   string `json:"image"`
	Storage string `json:"storage,omitempty"`
	Network string `json:"network,omitempty"`
	// Memory is in MiB and Disk in GiB.
	Memory int  `json:"memory,omitempty"`
	Cores  int  `json:"cores,omitempty"`
	Disk   int  `json:"disk,omitempty"`
	Start  bool `json:"start,omitempty"`
}

// Create is shippin.infrastructure.server.guest.create: POST /v1/operator/infrastructure/servers/{serverId}/guests.
var Create = mcpl.Def[CreateInput, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Make a virtual machine or container on a server from an image.", Access: mcpl.Staff, Perm: "infra.guests.manage"},
	Run: func(c *mcpl.Call, in CreateInput) (hv.Guest, error) {
		if !mcpl.Valid(in.Kind) {
			return hv.Guest{}, mcpl.Invalid("Kind is vm or ct.")
		}
		if strings.TrimSpace(in.Name) == "" {
			return hv.Guest{}, mcpl.Invalid("A new guest needs a name.")
		}
		if in.Memory < 0 || in.Cores < 0 || in.Disk < 0 {
			return hv.Guest{}, mcpl.Invalid("Sizes are whole numbers, zero or more.")
		}
		h, ep, d, err := server.Open(c, in.Server)
		if err != nil {
			return hv.Guest{}, err
		}
		spec, known := hv.SpecFor(ep.Kind)
		if !known {
			return hv.Guest{}, mcpl.Conflict("This kind of server does not make guests.")
		}
		if err := placement(in, spec.CreateFields); err != nil {
			return hv.Guest{}, err
		}
		m, ok := d.(hv.GuestManager)
		if !ok {
			return hv.Guest{}, mcpl.Conflict("This kind of server does not make guests.")
		}
		guest := hv.GuestSpec{Kind: string(in.Kind), Name: in.Name, Image: in.Image, Storage: in.Storage, Network: in.Network,
			Memory: in.Memory, Cores: in.Cores, Disk: in.Disk, Start: in.Start}
		if alias, from, protocol, ok := h.ImageSource(c.Ctx, ep.Kind, in.Image); ok {
			guest.Image, guest.ImageServer, guest.ImageProtocol = alias, from, protocol
		}
		g, err := h.Track(c.Ctx, operation(c, in.Server, in.Name, "create"), func(ctx context.Context) (hv.Guest, error) {
			return m.CreateGuest(ctx, guest)
		})
		return g, server.Problem(err)
	},
}

// placement checks storage, network and disk against what this kind of server takes when it makes a
// guest: each one it takes must be chosen, and one it does not take must be left out.
func placement(in CreateInput, fields []string) error {
	for _, f := range []struct{ name, value string }{{"storage", in.Storage}, {"network", in.Network}} {
		takes := slices.Contains(fields, f.name)
		if takes && strings.TrimSpace(f.value) == "" {
			return mcpl.Invalid("Choose a " + f.name + " for this guest.")
		}
		if !takes && f.value != "" {
			return mcpl.Invalid("This kind of server takes no " + f.name + " for a new guest; leave it out.")
		}
	}
	if slices.Contains(fields, "disk") && in.Disk == 0 {
		return mcpl.Invalid("Choose a disk size in GiB.")
	}
	return nil
}
