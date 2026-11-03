package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AgentReq starts the build agent: provider is an id from agent.providers (an enum, never a
// command), resume continues the last conversation instead of starting from the prompt.
type AgentReq struct {
	Provider string `json:"provider"`
	Resume   bool   `json:"resume"`
}

// AgentStatus is the workspace's report on its agent: booleans, a timestamp and a count only.
type AgentStatus struct {
	Session      bool  `json:"session"`
	Activity     int64 `json:"activity"`
	Exited       bool  `json:"exited"`
	ExitCode     int   `json:"exit_code"`
	FilesChanged int   `json:"files_changed"`
}

// asUser is the exec prefix that runs a command as the agent user with their login environment.
func (s *Server) asUser(inst string) []string {
	a := s.cfg.Agent
	uid := fmt.Sprint(a.UID)
	return []string{"exec", inst, "--user", uid, "--group", uid, "--cwd", "/home/" + a.User,
		"--env", "HOME=/home/" + a.User, "--env", "USER=" + a.User, "--env", "SESSION=" + a.Session, "--"}
}

func (s *Server) agentPlace(ctx context.Context, t, w string) (place, error) {
	if !s.cfg.Agent.enabled() {
		return place{}, fmt.Errorf("build agents are not configured")
	}
	return s.placeOf(ctx, t, w)
}

// PutPrompt writes the prompt to ~/.shippin/prompt.md as the user, mode 0600. The text travels on
// stdin only: it is never part of any argv hostd runs.
func (s *Server) PutPrompt(ctx context.Context, t, w, prompt string) error {
	if strings.TrimSpace(prompt) == "" || len(prompt) > s.cfg.Agent.PromptMaxBytes {
		return fmt.Errorf("prompt must be 1-%d bytes", s.cfg.Agent.PromptMaxBytes)
	}
	at, err := s.agentPlace(ctx, t, w)
	if err != nil {
		return err
	}
	args := append(s.asUser(Instance(w)), "sh", "-c",
		`umask 077 && mkdir -p "$HOME/.shippin" && cat > "$HOME/.shippin/prompt.md.tmp" && mv -f "$HOME/.shippin/prompt.md.tmp" "$HOME/.shippin/prompt.md"`)
	if _, err := at.inc(ctx, prompt, args...); err != nil {
		return fmt.Errorf("prompt not written")
	}
	return nil
}

// StartAgent installs the agent files and starts `shippin-agent <provider>` in a detached tmux
// session as the user. A session that already exists is left alone (idempotent).
func (s *Server) StartAgent(ctx context.Context, t, w string, r AgentReq) (bool, error) {
	if _, ok := s.cfg.Agent.Providers[r.Provider]; !ok {
		return false, fmt.Errorf("unknown agent provider")
	}
	at, err := s.agentPlace(ctx, t, w)
	if err != nil {
		return false, err
	}
	inst := Instance(w)
	if s.cfg.Agent.sessions() {
		if err := s.ensureSessions(ctx, at, inst); err != nil {
			return false, err
		}
		return s.startProviderWindow(ctx, at, inst, r)
	}
	if err := s.installAgent(ctx, at, inst); err != nil {
		return false, err
	}
	u := s.asUser(inst)
	if at.has(ctx, append(u, "tmux", "has-session", "-t", s.cfg.Agent.Session)...) {
		return false, nil
	}
	cmd := []string{"tmux", "new-session", "-d", "-s", s.cfg.Agent.Session, "-x", "220", "-y", "50", agentWrapperPath, r.Provider}
	if r.Resume {
		cmd = append(cmd, "--resume")
	}
	if _, err := at.do(ctx, append(u, cmd...)...); err != nil {
		return false, fmt.Errorf("agent did not start")
	}
	return true, nil
}

// startProviderWindow runs the provider CLI in its own tmux window. A window already running
// that CLI is left alone (idempotent); resume does not kill a live agent.
func (s *Server) startProviderWindow(ctx context.Context, at place, inst string, r AgentReq) (bool, error) {
	a := s.cfg.Agent
	u := s.asUser(inst)
	target := a.Session + ":" + r.Provider
	out, err := at.do(ctx, append(u, "tmux", "display-message", "-p", "-t", target, "#{pane_current_command}")...)
	cli := a.Providers[r.Provider].Run[0]
	if cmd := strings.TrimSpace(out); err == nil && (cmd == "shippin-agent" || cmd == cli) {
		return false, nil
	}
	argv := []string{"tmux", "respawn-window", "-k", "-t", target, "--", agentWrapperPath, r.Provider}
	if r.Resume {
		argv = append(argv, "--resume")
	}
	if _, err := at.do(ctx, append(u, argv...)...); err != nil {
		return false, fmt.Errorf("agent did not start")
	}
	return true, nil
}

// ReadAgentStatus runs the fixed status script as the user and decodes it into typed values.
func (s *Server) ReadAgentStatus(ctx context.Context, t, w string) (*AgentStatus, error) {
	at, err := s.agentPlace(ctx, t, w)
	if err != nil {
		return nil, err
	}
	out, err := at.do(ctx, append(s.asUser(Instance(w)), "bash", "-c", agentStatusScript)...)
	if err != nil {
		return nil, fmt.Errorf("agent status unavailable")
	}
	if len(out) > 1024 {
		return nil, fmt.Errorf("agent status too large")
	}
	var st AgentStatus
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &st) != nil {
		return nil, fmt.Errorf("agent status malformed")
	}
	return &st, nil
}
