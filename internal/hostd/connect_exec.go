package hostd

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
)

// userExec is the incus exec prefix that runs argv as the image user with the CLI homes set
// (connect.env). argv comes from config only; nothing user-supplied or secret is ever on it.
func (s *Server) userExec(inst string, interactive bool, argv ...string) []string {
	a, c := s.cfg.AI, s.cfg.Connect
	mode := "--force-noninteractive"
	if interactive {
		mode = "--force-interactive"
	}
	args := []string{"exec", mode, inst, "--user", fmt.Sprint(a.UID), "--group", fmt.Sprint(a.UID),
		"--env", "HOME=/home/" + a.User, "--env", "TERM=xterm-256color"}
	keys := make([]string, 0, len(c.Env))
	for k := range c.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--env", k+"="+c.Env[k])
	}
	return append(append(args, "--", "bash", "-l", "-c", `exec "$@"`, "shippin"), argv...)
}

// streamCmd is the seam for long-running execs (logins, terminals): tests swap in a local command.
var streamCmd = func(ctx context.Context, at place, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, "incus", at.argv(args)...)
}

// connectProvider resolves a configured provider or fails closed.
func (s *Server) connectProvider(id string) (ConnectProvider, error) {
	if !s.cfg.Connect.enabled() {
		return ConnectProvider{}, fmt.Errorf("connect accounts is not configured")
	}
	p, ok := s.cfg.Connect.Providers[id]
	if !ok {
		return ConnectProvider{}, fmt.Errorf("unknown provider")
	}
	return p, nil
}
