package hostd

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

// In-workspace files for the M4 build agent. They are pushed by hostd at agent start, so existing
// -ai1 workspaces get them without an image rebuild (the image build installs the same files).
var (
	//go:embed assets/shippin-agent
	agentWrapper string
	//go:embed assets/shippin-agent-term.service
	agentTermUnit string
	//go:embed assets/agent-status.sh
	agentStatusScript string
)

const (
	agentWrapperPath = "/usr/local/bin/shippin-agent"
	agentCmdDir      = "/etc/shippin/agent"
	agentTermUnitN   = "shippin-agent-term.service"
	agentTermEnv     = "/etc/default/shippin-agent-term"
)

// pushFile writes content (stdin, never argv) to path inside inst as root with mode, atomically.
func pushFile(ctx context.Context, at place, inst, path, mode, content string) error {
	dir := path[:strings.LastIndex(path, "/")]
	_, err := at.inc(ctx, content, "exec", inst, "--env", "F="+path, "--env", "D="+dir, "--env", "M="+mode, "--",
		"sh", "-c", `mkdir -p "$D" && cat > "$F.tmp" && chmod "$M" "$F.tmp" && mv -f "$F.tmp" "$F"`)
	return err
}

// installAgent pushes the wrapper, every provider's argv files and (when configured) the
// browser terminal unit, then (re)starts that unit. Idempotent.
func (s *Server) installAgent(ctx context.Context, at place, inst string) error {
	a := s.cfg.Agent
	if err := pushFile(ctx, at, inst, agentWrapperPath, "0755", agentWrapper); err != nil {
		return fmt.Errorf("agent wrapper not installed")
	}
	for p, v := range a.Providers {
		for mode, argv := range map[string][]string{"run": v.Run, "resume": v.Resume} {
			if err := pushFile(ctx, at, inst, agentCmdDir+"/"+p+"."+mode, "0644", strings.Join(argv, "\n")+"\n"); err != nil {
				return fmt.Errorf("agent command not installed")
			}
		}
	}
	if a.TermPort == 0 {
		return nil
	}
	env := fmt.Sprintf("AGENT_TERM_PORT=%d\nAGENT_TERM_IFACE=%s\nAGENT_TERM_BASE=%s\nAGENT_SESSION=%s\n",
		a.TermPort, a.TermIface, a.TermBase, a.Session)
	if err := pushFile(ctx, at, inst, agentTermEnv, "0644", env); err != nil {
		return fmt.Errorf("agent terminal not configured")
	}
	if err := pushFile(ctx, at, inst, "/etc/systemd/system/"+agentTermUnitN, "0644", agentTermUnit); err != nil {
		return fmt.Errorf("agent terminal unit not installed")
	}
	_, _ = at.do(ctx, "exec", inst, "--", "systemctl", "daemon-reload")
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "enable", "--now", agentTermUnitN); err != nil {
		return fmt.Errorf("agent terminal did not start")
	}
	return nil
}
