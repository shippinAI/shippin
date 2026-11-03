package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

type sshConn struct{ cl *ssh.Client }

type tools struct {
	c     creds
	mu    sync.Mutex
	conns map[string]*sshConn
}

func (t *tools) close() {
	for _, s := range t.conns {
		s.cl.Close()
	}
}

// client reuses an SSH connection per workspace and reconnects (new certificate) when it died.
func (t *tools) client(ref string) (*ssh.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.conns[ref]; ok {
		if _, _, err := s.cl.SendRequest("keepalive@shippin", true, nil); err == nil {
			return s.cl, nil
		}
		s.cl.Close()
	}
	cl, _, err := connect(t.c, ref)
	if err != nil {
		return nil, err
	}
	t.conns[ref] = &sshConn{cl}
	return cl, nil
}

func (t *tools) run(ref, cmd string, stdin []byte) (string, string, int, error) {
	cl, err := t.client(ref)
	if err != nil {
		return "", "", -1, err
	}
	s, err := cl.NewSession()
	if err != nil {
		return "", "", -1, err
	}
	defer s.Close()
	var o, e bytes.Buffer
	s.Stdout, s.Stderr, s.Stdin = &o, &e, bytes.NewReader(stdin)
	code := 0
	if err := s.Run(cmd); err != nil {
		var x *ssh.ExitError
		if !errors.As(err, &x) {
			return "", "", -1, err
		}
		code = x.ExitStatus()
	}
	return o.String(), e.String(), code, nil
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

const resumeScript = `tmux ls 2>/dev/null | sed 's/^/tmux: /'; for g in $(find ~ -maxdepth 3 -name .git -type d 2>/dev/null); do d=${g%/.git}; echo "repo: $d [$(git -C "$d" branch --show-current)] $(git -C "$d" log -1 --format='%h %s (%cr)' 2>/dev/null); $(git -C "$d" status --porcelain | wc -l) uncommitted"; done`

func (t *tools) call(name string, raw json.RawMessage) (string, error) {
	var a struct{ Workspace, Command, Cwd, Path, Content string }
	_ = json.Unmarshal(raw, &a)
	switch name {
	case "list_workspaces":
		ws, email, err := workspaces(t.c)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(map[string]any{"account": email, "workspaces": ws}, "", "  ")
		return string(b), nil
	case "exec":
		cmd := a.Command
		if a.Cwd != "" {
			cmd = "cd " + quote(a.Cwd) + " && " + cmd
		}
		o, e, code, err := t.run(a.Workspace, cmd, nil)
		return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, o, e), err
	case "read_file":
		o, e, code, err := t.run(a.Workspace, "head -c 1048576 -- "+quote(a.Path), nil)
		if err == nil && code != 0 {
			err = errors.New(strings.TrimSpace(e))
		}
		return o, err
	case "write_file":
		_, e, code, err := t.run(a.Workspace, `mkdir -p "$(dirname -- `+quote(a.Path)+`)" && cat > `+quote(a.Path), []byte(a.Content))
		if err == nil && code != 0 {
			err = errors.New(strings.TrimSpace(e))
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path), err
	case "resume":
		o, _, _, err := t.run(a.Workspace, resumeScript, nil)
		return "connected to " + a.Workspace + "\n" + o, err
	}
	return "", fmt.Errorf("unknown tool %s", name)
}
