package hostd

import (
	"context"
	"fmt"
	"log"
)

// migrateNetwork moves a tenant from the legacy LAN uplink to the transit uplink:
// stop instances, detach NICs, drop forwards/peers/network, recreate on transit, restart.
// Volumes and instances are kept; workspace IPs change (the console re-syncs routes/ports).
func (s *Server) migrateNetwork(ctx context.Context, t string) error {
	pr := "--project=" + Project(t)
	log.Printf("migrate %s: moving %s to %s", t, Network(t), s.cfg.Transit)
	var insts []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/instances?project=%s&recursion=1", Project(t)), &insts); err != nil {
		return err
	}
	running := []string{}
	for _, i := range insts {
		if i.Status == "Running" {
			running = append(running, i.Name)
			if _, err := incus(ctx, "stop", i.Name, pr, "--force"); err != nil {
				return err
			}
		}
		_, _ = incus(ctx, "config", "device", "remove", i.Name, "eth0", pr)
	}
	_, _ = incus(ctx, "profile", "device", "remove", "default", "eth0", pr)
	if err := s.dropNetwork(ctx, t); err != nil {
		return err
	}
	if err := s.createTenantNet(ctx, t, pr); err != nil {
		return err
	}
	if err := ignoreExists(incus(ctx, "profile", "device", "add", "default", "eth0", "nic", pr,
		"network="+Network(t), "name=eth0")); err != nil {
		return err
	}
	for _, n := range running {
		if _, err := incus(ctx, "start", n, pr); err != nil && !isRunning(err) {
			return err
		}
	}
	return nil
}

// dropNetwork deletes the tenant network with its forwards and peers (both sides).
func (s *Server) dropNetwork(ctx context.Context, t string) error {
	pr, pp := "--project="+Project(t), "--project="+s.cfg.Platform
	var fws []struct {
		ListenAddress string `json:"listen_address"`
	}
	_ = query(ctx, fmt.Sprintf("/1.0/networks/%s/forwards?project=%s&recursion=1", Network(t), Project(t)), &fws)
	for _, f := range fws {
		_, _ = incus(ctx, "network", "forward", "delete", Network(t), f.ListenAddress, pr)
	}
	var peers []struct {
		Name string `json:"name"`
	}
	_ = query(ctx, fmt.Sprintf("/1.0/networks/%s/peers?project=%s&recursion=1", Network(t), Project(t)), &peers)
	for _, p := range peers {
		_, _ = incus(ctx, "network", "peer", "delete", Network(t), p.Name, pr)
	}
	for _, hub := range []string{s.cfg.Edge.PlatformNetwork, s.cfg.Edge.HubNetwork} {
		_, _ = incus(ctx, "network", "peer", "delete", hub, Project(t), pp)
	}
	if !exists(ctx, "network", "show", Network(t), pr) {
		return nil
	}
	_, err := incus(ctx, "network", "delete", Network(t), pr)
	return err
}
