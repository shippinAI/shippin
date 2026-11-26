// Package guest is what runs on a server: a virtual machine or a container, as the server's hypervisor
// reports it. Its actions are the other files in this folder (docs/mcpl.md). Every change is recorded
// in the server's operations before it runs, and the answer is the guest as the hypervisor reported it
// afterwards, never a state the panel assumed.
package guest

import (
	"context"
	"os"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// OnServer names the server from the route, for a list or a create.
type OnServer struct {
	Server string `path:"serverId" json:"-"`
}

// ByID names one guest on one server from the route.
type ByID struct {
	Server string `path:"serverId" json:"-"`
	ID     string `path:"guestId" json:"-"`
}

// PowerInput is a power change to one guest.
type PowerInput struct {
	ByID
	// ConfirmSelf must be true to stop or restart the guest this panel itself runs on.
	ConfirmSelf bool `json:"confirm_self,omitempty"`
	// Review is the id of the outside review requested for this change, on an installation that sends
	// power changes to a reviewer.
	Review string `json:"review,omitempty"`
}

// Self is the guest this panel runs on, if it can tell. Stopping it takes the panel down, so nothing
// does that without being told to. An installation sets SHIPPIN_SELF_GUEST when the names differ.
func Self() string {
	if v := strings.TrimSpace(os.Getenv("SHIPPIN_SELF_GUEST")); v != "" {
		return v
	}
	h, _ := os.Hostname()
	return h
}

// IsSelf reports whether g is the guest this panel runs on.
func IsSelf(g hv.Guest) bool {
	self := Self()
	return self != "" && (strings.EqualFold(g.Name, self) || strings.EqualFold(g.ID, self))
}

// MarkSelf marks the panel's own guest in a list.
func MarkSelf(items []hv.Guest) []hv.Guest {
	for i := range items {
		items[i].Self = IsSelf(items[i])
	}
	return items
}

// GuardSelf refuses a change that would take down the panel's own guest, unless the person confirmed
// it and may. Starting a guest is never refused.
func GuardSelf(g hv.Guest, verb string, confirmed, may bool) error {
	switch {
	case verb == "start" || !IsSelf(g):
		return nil
	case !confirmed:
		return mcpl.Conflict("This panel runs on that guest. Doing this takes the panel down; confirm it to go ahead.")
	case !may:
		return mcpl.Forbidden("Changing the guest this panel runs on needs the " + server.PermSelf + " permission.")
	}
	return nil
}

// current reads the guest a change is about to touch and applies the self guard.
func current(c *mcpl.Call, d hv.Driver, id, verb string, confirmSelf bool) (hv.Guest, error) {
	ctx, cancel := context.WithTimeout(c.Ctx, 10*time.Second)
	defer cancel()
	g, err := d.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, server.Problem(err)
	}
	if g.ID != id {
		return hv.Guest{}, mcpl.Conflict("The server answered with a different guest than the one asked for.")
	}
	return g, GuardSelf(g, verb, confirmSelf, c.Can(server.PermSelf))
}

// operation is the record of one change, as the server's operations keep it.
func operation(c *mcpl.Call, serverID, guest, verb string) servers.Operation {
	return servers.Operation{ServerID: serverID, Guest: guest, Verb: verb, Actor: c.Actor, ActorName: c.Actor, Note: c.Reason}
}

// power runs one power verb: start, stop, shutdown or reboot.
func power(c *mcpl.Call, in PowerInput, verb string) (hv.Guest, error) {
	h, _, d, err := server.Open(c, in.Server)
	if err != nil {
		return hv.Guest{}, err
	}
	if h.OutsideReview(c.Ctx) {
		if in.Review == "" {
			return hv.Guest{}, mcpl.Conflict("This installation sends power changes to an outside reviewer. Request a review for this change first, then send its id as review.")
		}
		g, err := h.ReviewedPower(c.Ctx, reviewed(c, in, verb))
		return g, server.Problem(err)
	}
	if _, err := current(c, d, in.ID, verb, in.ConfirmSelf); err != nil {
		return hv.Guest{}, err
	}
	g, err := h.Track(c.Ctx, operation(c, in.Server, in.ID, verb), func(ctx context.Context) (hv.Guest, error) {
		switch verb {
		case "start":
			return d.Start(ctx, in.ID)
		case "stop":
			return d.Stop(ctx, in.ID)
		case "shutdown":
			return d.Shutdown(ctx, in.ID)
		}
		return d.Reboot(ctx, in.ID)
	})
	return g, server.Problem(err)
}

// reviewed is a power change as the outside reviewer's path takes it.
func reviewed(c *mcpl.Call, in PowerInput, verb string) ports.ReviewedPower {
	return ports.ReviewedPower{Server: in.Server, Guest: in.ID, Verb: verb, Review: in.Review, Actor: c.Actor, Reason: c.Reason,
		ConfirmSelf: in.ConfirmSelf, MaySelf: c.Can(server.PermSelf)}
}
