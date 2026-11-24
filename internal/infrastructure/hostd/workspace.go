package hostd

import (
	"context"
	"fmt"
)

type WorkspaceReq struct {
	Name    string   `json:"name"`
	Plan    string   `json:"plan"`
	Keys    []string `json:"ssh_public_keys"`
	Port    int      `json:"ssh_port"`
	SSHIP   string   `json:"ssh_ip"`
	Creator string   `json:"created_by"`
	Host    string   `json:"host"`
	Domains []string `json:"domains"`
	Rules   []Rule   `json:"firewall"`
	// Template fields (empty for legacy workspaces): image alias, size override, repo, startup.
	Image    string    `json:"image"`
	Size     Size      `json:"size"`
	Repo     string    `json:"repo"`
	Startup  string    `json:"startup"`
	Previews []Preview `json:"previews"`
	// Project is the Shippin project UUID (M1); a VM tenant places it in inner project pj-<uuid>.
	Project string `json:"project_id"`
	// Environment and Boundary are labels only (user.shippin.*), so Incus can be reconciled.
	Environment string `json:"environment_id"`
	Boundary    string `json:"boundary_id"`
	// AIHome is the creator's account id when they have an AI home here (M2); empty = none.
	AIHome string `json:"ai_home_account"`
	// GitToken is set for GitHub imports (M3): installed before the clone, never logged.
	GitToken *GitToken `json:"git_token,omitempty"`
}

// CreateWorkspace launches w-<uuid> with home volume h-<uuid>, installs keys, adds the SSH port.
func (s *Server) CreateWorkspace(ctx context.Context, t, w string, r WorkspaceReq) (string, error) {
	if r.AIHome != "" {
		return "", fmt.Errorf("provider credentials require an isolated owner runner")
	}
	plan, ok := s.cfg.Plans[r.Plan]
	if !ok {
		return "", fmt.Errorf("unknown plan")
	}
	keys, err := ValidKeys(r.Keys)
	if err != nil {
		return "", err
	}
	if err := ValidLabel(r.Name); err != nil {
		return "", err
	}
	if err := ValidPort(r.Port, s.cfg.PortMin, s.cfg.PortMax); err != nil {
		return "", err
	}
	if err := s.ValidHost(r.Host); err != nil {
		return "", err
	}
	if err := ValidUUID(r.Creator); err != nil && r.Creator != "" {
		r.Creator = ""
	}
	if err := s.validTemplate(r); err != nil {
		return "", err
	}
	capped := false
	if z := capSize(plan, r.Size); z != r.Size {
		r.Size, capped = z, true
	}
	at, err := s.placeFor(ctx, t, w, r.Project, plan)
	if err != nil {
		return "", err
	}
	inst := Instance(w)
	step := func(m string) { setProgress(w, m) }
	defer clearProgress(w)
	if capped {
		step("Capping size to the plan")
	}
	step("Creating home volume")
	if err := ignoreExists(at.do(ctx, "storage", "volume", "create", at.pool(), HomeVolume(w),
		fmt.Sprintf("size=%dGiB", plan.HomeGiB), "user.shippin.workspace="+w)); err != nil {
		return "", err
	}
	if !at.has(ctx, "info", inst) {
		if at.vm {
			step("Importing template into tenant VM")
			if err := s.ensureInnerImage(ctx, at, s.image(r)); err != nil {
				return "", err
			}
		}
		step("Launching from " + s.image(r))
		args := append(append([]string{"init", s.image(r), inst}, workspaceLabels(t, w, r)...),
			innerInitArgs(at, plan, r.Size)...)
		if _, err := at.do(ctx, args...); err != nil {
			return "", err
		}
		if _, err := at.do(ctx, "storage", "volume", "attach", at.pool(), HomeVolume(w), inst, "/home"); err != nil {
			return "", err
		}
	}
	if err := s.mountAIHome(ctx, at, inst, r.AIHome); err != nil {
		return "", err
	}
	step("Starting")
	if _, err := at.do(ctx, "start", inst); err != nil && !isRunning(err) {
		return "", err
	}
	step("Waiting for network")
	ip, err := at.waitIP(ctx, inst)
	if err != nil {
		return "", err
	}
	step("Installing SSH keys")
	if _, err := at.inc(ctx, keys, "exec", inst, "--", "/usr/local/sbin/shippin-ws-init"); err != nil {
		return "", err
	}
	if err := s.trustBridgeCA(ctx, at, inst, w); err != nil {
		return "", err
	}
	if err := s.ownAIHome(ctx, at, inst, r.AIHome); err != nil {
		return "", err
	}
	if err := s.pushGitToken(ctx, at, inst, r.GitToken); err != nil {
		return "", err
	}
	if err := s.ensureSessions(ctx, at, inst); err != nil {
		return "", err
	}
	if err := s.bootstrap(ctx, at, w, r, step); err != nil {
		return "", err
	}
	step("Publishing HTTPS routes")
	return ip, s.publish(ctx, at, w, ip, r)
}

// publish pins the workspace IP, then (re)publishes its HTTPS route and shared-endpoint ports.
func (s *Server) publish(ctx context.Context, at place, w, ip string, r WorkspaceReq) error {
	inst := Instance(w)
	if err := at.checkIP(ip); err != nil {
		return err
	}
	_ = ignoreExists(at.do(ctx, "config", "device", "override", inst, "eth0", "ipv4.address="+ip))
	_, _ = at.do(ctx, "config", "device", "set", inst, "eth0", "ipv4.address="+ip)
	if err := s.PublishRoute(ctx, w, r.Host, ip, r.Domains); err != nil {
		return err
	}
	if err := s.SyncPreviews(ctx, w, r.Host, ip, r.Previews); err != nil {
		return err
	}
	return s.SyncFirewall(ctx, at.t, w, ip, r.Port, r.Rules)
}
