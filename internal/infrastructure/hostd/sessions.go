package hostd

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"strings"
)

// Phase 1 session manager. The script is pushed at ensure time, so existing images pick it
// up without a rebuild. The systemd unit is what brings the windows back after a reboot;
// ensureSessions also runs the script once so a start does not wait for the next boot.

//go:embed assets/shippin-sessions
var sessionsScript string

const (
	sessionsBin   = "/usr/local/bin/shippin-sessions"
	sessionsDir   = "/etc/shippin/sessions"
	sessionsUnitN = "shippin-sessions.service"
	sessionsEnv   = "/etc/default/shippin-sessions"
)

// ensureSessions installs and boots the session manager when agent.session_manager is on.
// It is a no-op otherwise, so the default config does not change workspace start.
func (s *Server) ensureSessions(ctx context.Context, at place, inst string) error {
	a := s.cfg.Agent
	if !a.sessions() {
		return nil
	}
	env := "SESSION=" + a.Session + "\n"
	if err := pushFile(ctx, at, inst, sessionsEnv, "0644", env); err != nil {
		return fmt.Errorf("session manager not configured")
	}
	if err := pushFile(ctx, at, inst, sessionsDir+"/providers", "0644", providerList(a)); err != nil {
		return fmt.Errorf("session manager providers not installed")
	}
	if err := pushFile(ctx, at, inst, sessionsDir+"/enabled", "0644", ""); err != nil {
		return fmt.Errorf("session manager not enabled")
	}
	if err := pushFile(ctx, at, inst, sessionsBin, "0755", sessionsScript); err != nil {
		return fmt.Errorf("session manager not installed")
	}
	if err := pushFile(ctx, at, inst, "/etc/systemd/system/"+sessionsUnitN, "0644", s.sessionsUnit()); err != nil {
		return fmt.Errorf("session manager unit not installed")
	}
	// Provider argv and the browser terminal, then the boot unit, then an immediate boot
	// so the windows exist before anything attaches.
	if err := s.installAgent(ctx, at, inst); err != nil {
		return err
	}
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("session manager did not reload")
	}
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "enable", "--now", sessionsUnitN); err != nil {
		return fmt.Errorf("session manager did not start")
	}
	if _, err := at.do(ctx, append(s.asUser(inst), sessionsBin, "boot")...); err != nil {
		return fmt.Errorf("session manager did not boot")
	}
	return nil
}

func providerList(a AgentConfig) string {
	names := make([]string, 0, len(a.Providers))
	for n := range a.Providers {
		names = append(names, n)
	}
	slices.Sort(names)
	return strings.Join(names, "\n") + "\n"
}

func (s *Server) sessionsUnit() string {
	u := s.cfg.Agent.User
	return strings.Join([]string{
		"[Unit]",
		"Description=Shippin session manager (tmux windows for each AI CLI)",
		"ConditionPathExists=" + sessionsDir + "/enabled",
		"After=network-online.target",
		"",
		"[Service]",
		"Type=oneshot",
		"RemainAfterExit=yes",
		"User=" + u,
		"Group=" + u,
		"WorkingDirectory=/home/" + u,
		"Environment=HOME=/home/" + u,
		"EnvironmentFile=" + sessionsEnv,
		"ExecStart=" + sessionsBin + " boot",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}
