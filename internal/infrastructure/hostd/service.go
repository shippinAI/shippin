package hostd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"
)

//go:embed assets/rrflow-state.service
var rrflowUnit string

// ServiceReq places a service instance: the Shippin project it belongs to and the tenant's plan
// (for the inner project's restrictions).
type ServiceReq struct {
	Project string `json:"project_id"`
	Plan    string `json:"plan"`
	// Shared: one instance per tenant VM in the VM-wide inner project, used by all its projects
	// (Notion D36); Project is then only the namespace being set up. False = pre-D36 per project.
	Shared bool `json:"shared"`
}

// ServiceView is where a running instance answers (inside the boundary VM's inner network).
type ServiceView struct {
	IPv4 string `json:"ipv4"`
	Port int    `json:"port"`
	URL  string `json:"url"`
}

// servicePlace is the project's inner Incus project on the tenant's boundary VM. Services need a
// boundary VM: legacy tenants have none, so they are refused.
func (s *Server) servicePlace(ctx context.Context, t, pj string, plan *Plan, shared bool) (place, error) {
	if s.cfg.Services.RRFlow == nil {
		return place{}, fmt.Errorf("rrflow/state is not configured")
	}
	if ValidUUID(pj) != nil {
		return place{}, fmt.Errorf("invalid project id")
	}
	at, err := s.newPlace(ctx, t)
	if err != nil {
		return place{}, err
	}
	if !at.vm {
		return place{}, fmt.Errorf("managed services need the tenant's boundary VM")
	}
	if shared {
		return at.in(""), nil // the VM-wide inner project shippin-tvm-init made, on the shared bridge
	}
	at = at.in(pj)
	if plan != nil {
		return at, s.ensureInnerProject(ctx, at, *plan)
	}
	return at, nil
}

// EnsureRRFlow creates (or repairs) si-<uuid> running rrflow-state, pushing this release's
// binary, and returns its address. Idempotent: reruns update the binary and restart the unit.
func (s *Server) EnsureRRFlow(ctx context.Context, t, si string, r ServiceReq) (*ServiceView, error) {
	plan, ok := s.cfg.Plans[r.Plan]
	if !ok {
		return nil, fmt.Errorf("unknown plan")
	}
	at, err := s.servicePlace(ctx, t, r.Project, &plan, r.Shared)
	if err != nil {
		return nil, err
	}
	c, inst := s.cfg.Services.RRFlow, ServiceInstance(si)
	bin, err := os.ReadFile(c.Binary)
	if err != nil || len(bin) == 0 {
		return nil, fmt.Errorf("rrflow-state binary missing on the host")
	}
	if !at.has(ctx, "info", inst) {
		if err := s.ensureInnerImage(ctx, at, c.Image); err != nil {
			return nil, err
		}
		owner := "user.shippin.project=" + r.Project
		if r.Shared {
			owner = "user.shippin.scope=vm"
		}
		args := append([]string{"init", c.Image, inst, "-c", "user.shippin.tenant=" + t, "-c", owner,
			"-c", "user.shippin.service_instance=" + si, "-c", "user.shippin.service=rrflow-state"}, innerInitArgs(at, plan, Size{})...)
		args = append(args, "-c", fmt.Sprintf("limits.cpu=%d", c.CPU), "-c", "limits.memory="+c.Memory)
		if _, err := at.do(ctx, args...); err != nil {
			return nil, err
		}
	}
	if _, err := at.do(ctx, "start", inst); err != nil && !isRunning(err) {
		return nil, err
	}
	ip, err := at.waitIP(ctx, inst)
	if err != nil {
		return nil, err
	}
	if err := at.checkIP(ip); err != nil {
		return nil, err
	}
	_, err = at.do(ctx, "exec", inst, "--env", "D="+c.DataDir, "--", "sh", "-c",
		`id rrflow >/dev/null 2>&1 || useradd --system --home-dir "$D" --shell /usr/sbin/nologin rrflow; install -d -o rrflow -g rrflow -m 0700 "$D"`)
	if err != nil {
		return nil, fmt.Errorf("rrflow user not created")
	}
	env := fmt.Sprintf("RRFLOW_LISTEN=:%d\nRRFLOW_DATA=%s\nRRFLOW_ORG=%s\n", c.Port, c.DataDir, t)
	for _, f := range [][3]string{{"/usr/local/bin/rrflow-state", "0755", string(bin)}, {"/etc/default/rrflow-state", "0644", env},
		{"/etc/systemd/system/rrflow-state.service", "0644", strings.ReplaceAll(rrflowUnit, "/var/lib/rrflow", c.DataDir)}} {
		if err := pushFile(ctx, at, inst, f[0], f[1], f[2]); err != nil {
			return nil, fmt.Errorf("rrflow-state not installed")
		}
	}
	_, _ = at.do(ctx, "exec", inst, "--", "systemctl", "daemon-reload")
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "enable", "rrflow-state.service"); err != nil {
		return nil, err
	}
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "restart", "rrflow-state.service"); err != nil {
		return nil, err
	}
	health := fmt.Sprintf("http://127.0.0.1:%d/healthz", c.Port)
	for i := 0; ; i++ {
		if at.has(ctx, "exec", inst, "--", "curl", "-sf", "-o", "/dev/null", health) {
			break
		}
		if i == 10 {
			return nil, fmt.Errorf("rrflow-state did not become healthy")
		}
		time.Sleep(time.Second)
	}
	return &ServiceView{IPv4: ip, Port: c.Port, URL: fmt.Sprintf("http://%s:%d", ip, c.Port)}, nil
}

// DeleteService stops and deletes si-<uuid> (its data goes with it).
func (s *Server) DeleteService(ctx context.Context, t, pj, si string, shared bool) error {
	at, err := s.servicePlace(ctx, t, pj, nil, shared)
	if err != nil {
		return err
	}
	_, err = at.do(ctx, "delete", ServiceInstance(si), "--force")
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}
