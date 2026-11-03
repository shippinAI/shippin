package hostd

import (
	"context"
	"encoding/json"
	"fmt"
)

// placeFor is where workspace w of Shippin project pj goes. A VM tenant gets one inner Incus
// project pj-<uuid> per Shippin project (M1 step 3), created on first use with the shared default
// profile, pool and images so the inner network and root disk stay the ones shippin-tvm-init set.
func (s *Server) placeFor(ctx context.Context, t, w, pj string, plan Plan) (place, error) {
	at, err := s.placeOf(ctx, t, w)
	if err != nil || !at.vm || pj == "" {
		return at, err
	}
	if err := ValidUUID(pj); err != nil {
		return place{}, fmt.Errorf("invalid project id")
	}
	at = at.in(pj)
	return at, s.ensureInnerProject(ctx, at, plan)
}

// ensureInnerProject creates pj-<uuid> restricted exactly like the legacy "ws" project
// (shippin-tvm-init): its own default profile (Incus refuses restricted=true on a project that
// shares the default project's profiles) with the pool root disk, the inner bridge NIC and an
// isolated idmap, then isolated idmaps only, managed disks/NICs, no device passthrough. A setup
// that fails part-way deletes the half-made project, so the next call starts clean.
func (s *Server) ensureInnerProject(ctx context.Context, at place, plan Plan) error {
	host := at.in("") // project create/set run from the legacy project; the new one is the argument
	_, err := host.do(ctx, "project", "create", at.project(),
		"-c", "features.images=false", "-c", "features.profiles=true",
		"-c", "features.storage.volumes=false", "-c", "user.shippin.project="+at.pj,
		"-c", "user.shippin.tenant="+at.t)
	if err != nil {
		return ignoreExists("", err)
	}
	steps := [][]string{
		{"profile", "device", "add", "default", "root", "disk", "path=/", "pool=" + InnerPool},
		{"profile", "device", "add", "default", "eth0", "nic", "network=" + innerBridge, "name=eth0"},
		{"profile", "set", "default", "security.idmap.isolated=true"},
	}
	for _, a := range steps {
		if _, err = at.do(ctx, a...); err != nil {
			break
		}
	}
	if err == nil {
		_, err = host.do(ctx, append([]string{"project", "set", at.project()}, innerRestrictions(plan)...)...)
	}
	if err != nil {
		_, _ = host.do(context.WithoutCancel(ctx), "project", "delete", at.project())
		return fmt.Errorf("inner project setup: %w", err)
	}
	return nil
}

// innerProjectOf finds which inner project holds w inside the tenant VM: a pj-<uuid> project
// (M1) or the legacy "ws". Guest output is untrusted: only a pj- name with a valid UUID is used.
func (s *Server) innerProjectOf(ctx context.Context, at place, w string) string {
	if ValidUUID(w) != nil {
		return ""
	}
	// The API, not `incus list`: the CLI refuses --all-projects next to place's --project flag.
	out, err := at.do(ctx, "query", "/1.0/instances?all-projects=true&recursion=1&filter=name+eq+"+Instance(w))
	if err != nil {
		return ""
	}
	var rows []struct {
		Name    string `json:"name"`
		Project string `json:"project"`
	}
	if json.Unmarshal([]byte(out), &rows) != nil {
		return ""
	}
	for _, r := range rows {
		if r.Name == Instance(w) && len(r.Project) > 3 && r.Project[:3] == "pj-" && ValidUUID(r.Project[3:]) == nil {
			return r.Project[3:]
		}
	}
	return ""
}
