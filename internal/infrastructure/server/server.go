// Package server is a server this installation has added: an Incus host, a Proxmox VE host or a plain
// machine, reached by its own address and credential. Its actions are the other files in this folder
// (docs/mcpl.md); what runs on a server is the guest entity in the folder below.
//
// The rows are kept by internal/infrastructure/servers, and the hypervisor behind each is reached
// through ports.Hypervisors.
package server

import (
	"errors"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// PermSelf is needed, with a confirmation, to change the guest the panel itself runs on.
const PermSelf = "infra.self"

// ByID names one server from the route.
type ByID struct {
	ID string `path:"serverId" json:"-"`
}

// Hosts is the installation's servers and hypervisors.
func Hosts(c *mcpl.Call) (ports.Hypervisors, error) { return mcpl.Need[ports.Hypervisors](c) }

// Open finds a server and connects to its hypervisor.
func Open(c *mcpl.Call, id string) (ports.Hypervisors, servers.Endpoint, hv.Driver, error) {
	h, err := Hosts(c)
	if err != nil {
		return nil, servers.Endpoint{}, nil, err
	}
	ep, err := h.Server(c.Ctx, id)
	if err != nil {
		return nil, servers.Endpoint{}, nil, Problem(err)
	}
	d, err := h.Open(ep)
	if err != nil {
		return nil, servers.Endpoint{}, nil, Problem(err)
	}
	return h, ep, d, nil
}

// Problem is a server's or a hypervisor's refusal as the person should read it. What the hypervisor
// itself said is passed on, because it is the only account of why it refused.
func Problem(err error) error {
	var known *mcpl.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &known):
		return err
	case errors.Is(err, servers.ErrNotFound):
		return mcpl.NotFound("There is no server with that id, or no hypervisor is attached to it.")
	case errors.Is(err, hv.ErrUnsupported), errors.Is(err, hv.ErrNoHypervisor):
		return mcpl.Conflict("This kind of server does not offer that.")
	case errors.Is(err, hv.ErrGuestNotFound), strings.Contains(err.Error(), "not in inventory"):
		return mcpl.NotFound("The server has no guest with that id.")
	}
	return &mcpl.Error{Status: http.StatusBadGateway, Message: err.Error()}
}
