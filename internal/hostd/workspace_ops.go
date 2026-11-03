package hostd

import (
	"context"
	"fmt"
	"strings"
)

// Action runs one lifecycle action on a workspace of tenant t.
func (s *Server) Action(ctx context.Context, t, w, action string, r WorkspaceReq) error {
	if action == "domains" || action == "previews" {
		if err := s.ValidHost(r.Host); err != nil {
			return err
		}
	}
	if !knownActions[action] {
		return fmt.Errorf("unknown action")
	}
	at, err := s.placeOf(ctx, t, w)
	if err != nil {
		return err
	}
	inst := Instance(w)
	switch action {
	case "start":
		if _, err := at.do(ctx, "start", inst); err != nil && !isRunning(err) {
			return err
		}
		return s.ensureSessions(ctx, at, inst)
	case "stop":
		_, err := at.do(ctx, "stop", inst, "--timeout=30", "--force")
		if err != nil && strings.Contains(err.Error(), "already stopped") {
			return nil
		}
		return err
	case "restart":
		if _, err := at.do(ctx, "restart", inst, "--timeout=30", "--force"); err != nil {
			return err
		}
		return s.ensureSessions(ctx, at, inst)
	case "keys":
		keys, err := ValidKeys(r.Keys)
		if err != nil {
			return err
		}
		_, err = at.inc(ctx, keys, "exec", inst, "--", "/usr/local/sbin/shippin-ws-init")
		return err
	case "domains":
		ip, err := at.waitIP(ctx, inst)
		if err != nil {
			return err
		}
		return s.PublishRoute(ctx, w, r.Host, ip, r.Domains)
	case "firewall":
		ip, err := at.waitIP(ctx, inst)
		if err != nil {
			return err
		}
		return s.SyncFirewall(ctx, t, w, ip, r.Port, r.Rules)
	case "previews":
		ip, err := at.waitIP(ctx, inst)
		if err != nil {
			return err
		}
		return s.SyncPreviews(ctx, w, r.Host, ip, r.Previews)
	case "sync":
		return s.sync(ctx, at, w, r)
	case "rebuild":
		return s.rebuild(ctx, at, w, r)
	case "delete":
		return s.remove(ctx, at, w)
	}
	return fmt.Errorf("unknown action")
}

var knownActions = map[string]bool{"start": true, "stop": true, "restart": true, "keys": true, "domains": true,
	"firewall": true, "previews": true, "sync": true, "rebuild": true, "delete": true}

// rebuild re-creates the root disk from the current image; the home volume is kept.
func (s *Server) rebuild(ctx context.Context, at place, w string, r WorkspaceReq) error {
	if err := s.validTemplate(r); err != nil {
		return err
	}
	if err := s.Action(ctx, at.t, w, "stop", r); err != nil {
		return err
	}
	if at.vm {
		if err := s.ensureInnerImage(ctx, at, s.image(r)); err != nil {
			return err
		}
	}
	if _, err := at.do(ctx, "rebuild", s.image(r), Instance(w)); err != nil {
		return err
	}
	_, err := s.CreateWorkspace(ctx, at.t, w, r)
	return err
}

// remove unpublishes the route, drops the SSH port, deletes the instance and its home volume.
func (s *Server) remove(ctx context.Context, at place, w string) error {
	if err := s.removeWorkspaceRoutes(ctx, w); err != nil {
		return err
	}
	if err := s.SetPorts(ctx, w, nil); err != nil {
		return err
	}
	if err := s.dropFirewall(ctx, at.t, w); err != nil {
		return err
	}
	present, err := at.cleanupContains(ctx, "/1.0/instances?recursion=1", Instance(w))
	if err != nil {
		return err
	}
	if present {
		if _, err := at.do(ctx, "delete", Instance(w), "--force"); err != nil {
			return err
		}
	}
	present, err = at.cleanupContains(ctx, "/1.0/storage-pools/"+at.pool()+"/volumes/custom?recursion=1", HomeVolume(w))
	if err != nil {
		return err
	}
	if present {
		if _, err := at.do(ctx, "storage", "volume", "delete", at.pool(), HomeVolume(w)); err != nil {
			return err
		}
	}
	return nil
}

// sync re-pins the IP and republishes route + ports (after a network migration). A stopped
// workspace is started for the address and stopped again.
func (s *Server) sync(ctx context.Context, at place, w string, r WorkspaceReq) error {
	inst := Instance(w)
	if err := s.ValidHost(r.Host); err != nil {
		return err
	}
	var st struct {
		Status string `json:"status"`
	}
	if err := at.query(ctx, "/1.0/instances/"+inst, &st); err != nil {
		return err
	}
	if st.Status != "Running" {
		if _, err := at.do(ctx, "start", inst); err != nil && !isRunning(err) {
			return err
		}
		defer func() { _, _ = at.do(ctx, "stop", inst, "--force") }()
	}
	ip, err := at.waitIP(ctx, inst)
	if err != nil {
		return err
	}
	return s.publish(ctx, at, w, ip, r)
}
