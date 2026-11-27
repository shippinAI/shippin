// Package host_monitor is a server's host monitor: the small guest the panel makes, on a kind of server
// whose own API cannot say how busy the host is now, to read the host's load, CPU and memory through
// (internal/hv/incus/host_monitor.go). There is one per server at most, and none until someone turns it
// on. Its actions are the other files in this folder (docs/mcpl.md).
package host_monitor

import (
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// OnServer names the server from the route.
type OnServer struct {
	Server string `path:"serverId" json:"-"`
}

// State is the monitor on one server.
type State struct {
	// Supported is false for a kind of server that reports its own load and needs no monitor.
	Supported bool `json:"supported"`
	hv.HostMonitorState
}

// monitored opens the server and answers its monitor, or ok=false for a kind that has none.
func monitored(c *mcpl.Call, id string) (hv.HostMonitored, bool, error) {
	_, _, d, err := server.Open(c, id)
	if err != nil {
		return nil, false, err
	}
	m, ok := d.(hv.HostMonitored)
	return m, ok, nil
}
