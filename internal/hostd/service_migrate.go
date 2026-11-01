package hostd

import (
	"context"
	"fmt"
	"strings"
)

// rrflowFiles are rrflow/state's data files (internal/rrflow/state); bindings.json is not moved:
// workspaces are re-bound to the shared instance with fresh tokens.
var rrflowFiles = []string{"sessions.jsonl", "memory.jsonl", "decisions.jsonl"}

// MigrateSnapshot is the Incus snapshot of a source instance taken before its state is copied.
func MigrateSnapshot(to string) string { return "gc-premigrate-" + to[:8] }

// MigrateRRFlow copies the state of pre-D36 instance `from` (inner project pj-<pj>) into
// namespace pj of the tenant VM's shared instance `to`. It snapshots `from` first and never
// deletes anything (RetireRRFlow does that, in a separate run, after the target verifies).
// Idempotent: a namespace that already has data is never overwritten.
func (s *Server) MigrateRRFlow(ctx context.Context, t, pj, from, to string) error {
	c := s.cfg.Services.RRFlow
	if c == nil || !c.Migrate.Enabled {
		return fmt.Errorf("rrflow migrate is disabled (services.rrflow_state.migrate.enabled)")
	}
	if ValidUUID(from) != nil || ValidUUID(to) != nil || from == to {
		return fmt.Errorf("invalid instance ids")
	}
	old, vm, err := s.migratePlaces(ctx, t, pj)
	if err != nil {
		return err
	}
	src, dst := ServiceInstance(from), ServiceInstance(to)
	if !old.has(ctx, "info", src) {
		return nil // nothing (left) to move
	}
	if !vm.has(ctx, "info", dst) {
		return fmt.Errorf("shared rrflow instance is not running yet")
	}
	if !old.has(ctx, "snapshot", "show", src, MigrateSnapshot(to)) {
		if _, err := old.do(ctx, "snapshot", "create", src, MigrateSnapshot(to)); err != nil {
			return fmt.Errorf("snapshot of the old rrflow instance failed")
		}
	}
	ns := c.DataDir + "/ns/" + pj
	if !vm.has(ctx, "exec", dst, "--env", "D="+ns, "--", "sh", "-c", `! ls "$D"/*.jsonl >/dev/null 2>&1`) {
		return nil // copied on an earlier run
	}
	if _, err := old.do(ctx, "start", src); err != nil && !isRunning(err) {
		return fmt.Errorf("old rrflow instance does not start")
	}
	for _, f := range rrflowFiles {
		body, err := old.do(ctx, "exec", src, "--env", "F="+c.DataDir+"/"+f, "--", "sh", "-c", `[ ! -e "$F" ] || cat "$F"`)
		if err != nil {
			return fmt.Errorf("old rrflow state unreadable")
		}
		if strings.TrimSpace(body) == "" {
			continue
		}
		if err := pushFile(ctx, vm, dst, ns+"/"+f+".import", "0600", body); err != nil {
			return fmt.Errorf("rrflow state not copied")
		}
	}
	// Publish all files at once, owned by the service user, so a half copy is never served.
	_, err = vm.do(ctx, "exec", dst, "--env", "D="+ns, "--", "sh", "-c",
		`mkdir -p "$D" && cd "$D" && for f in *.import; do [ -e "$f" ] && mv -f "$f" "${f%.import}"; done; chown rrflow:rrflow "${D%/*}"; chown -R rrflow:rrflow "$D"; chmod 0700 "$D"`)
	if err != nil {
		return fmt.Errorf("rrflow state not published")
	}
	_, _ = vm.do(ctx, "exec", dst, "--", "systemctl", "restart", "rrflow-state.service")
	return nil
}

// RetireRRFlow deletes pre-D36 instance `from` only when retire_old is on, its pre-migrate
// snapshot exists and namespace pj of `to` passes rrflow-state verify (decision chain intact).
func (s *Server) RetireRRFlow(ctx context.Context, t, pj, from, to string) error {
	c := s.cfg.Services.RRFlow
	if c == nil || !c.Migrate.RetireOld {
		return fmt.Errorf("retiring old rrflow instances is disabled (services.rrflow_state.migrate.retire_old)")
	}
	if ValidUUID(from) != nil || ValidUUID(to) != nil || from == to {
		return fmt.Errorf("invalid instance ids")
	}
	old, vm, err := s.migratePlaces(ctx, t, pj)
	if err != nil {
		return err
	}
	if !old.has(ctx, "info", ServiceInstance(from)) {
		return nil
	}
	if !old.has(ctx, "snapshot", "show", ServiceInstance(from), MigrateSnapshot(to)) {
		return fmt.Errorf("no pre-migrate snapshot; run migrate-to first")
	}
	if !vm.has(ctx, "exec", ServiceInstance(to), "--", "test", "-d", c.DataDir+"/ns/"+pj) {
		return fmt.Errorf("target namespace missing; old instance kept")
	}
	if _, err := vm.do(ctx, "exec", ServiceInstance(to), "--env", "RRFLOW_DATA="+c.DataDir, "--env", "RRFLOW_ORG="+t, "--",
		"runuser", "-u", "rrflow", "--", "/usr/local/bin/rrflow-state", "verify", pj); err != nil {
		return fmt.Errorf("target namespace does not verify; old instance kept")
	}
	return s.DeleteService(ctx, t, pj, from, false)
}

func (s *Server) migratePlaces(ctx context.Context, t, pj string) (place, place, error) {
	old, err := s.servicePlace(ctx, t, pj, nil, false)
	if err != nil {
		return place{}, place{}, err
	}
	vm, err := s.servicePlace(ctx, t, pj, nil, true)
	return old, vm, err
}
