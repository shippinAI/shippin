package hostd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"slices"
	"time"
)

// ChatConfig: named commands the console may run in a workspace as the workspace user, for the
// chat (the user's own AI CLI, signed in with their own subscription). The console only names a
// command; argv comes from here. The prompt arrives on stdin, stdout streams back as it comes.
type ChatConfig struct {
	User           string              `json:"user"` // workspace login user, e.g. dev
	Commands       map[string][]string `json:"commands"`
	MaxPromptBytes int64               `json:"max_prompt_bytes"`
	TimeoutSeconds int                 `json:"timeout_seconds"`
	// SessionCommands get the console's chat session id (a validated UUID, or "-") appended as
	// their last argument, so an in-workspace wrapper can key rrflow/state by it.
	SessionCommands []string `json:"session_commands"`
}

var chatSessionRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// chatArgv is the whitelisted argv for cmd, plus the session id for session commands.
func (c ChatConfig) chatArgv(cmd, session string) ([]string, bool) {
	argv, ok := c.Commands[cmd]
	if !ok || len(argv) == 0 {
		return nil, false
	}
	argv = append([]string(nil), argv...)
	if slices.Contains(c.SessionCommands, cmd) {
		if !chatSessionRe.MatchString(session) {
			session = "-"
		}
		argv = append(argv, session)
	}
	return argv, true
}

// chatExec (POST /tenants/{t}/workspaces/{w}/chat/{cmd}) streams the command's stdout, then one
// trailer line {"shippin_exit":N,"stderr":"…"} (stderr bounded to 4 KiB).
func (s *Server) chatExec(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.Chat
	argv, ok := c.chatArgv(r.PathValue("cmd"), r.URL.Query().Get("session"))
	if !ok || !safeName.MatchString(c.User) {
		fail(w, http.StatusNotFound, "unknown chat command")
		return
	}
	prompt, err := io.ReadAll(io.LimitReader(r.Body, c.MaxPromptBytes+1))
	if err != nil || int64(len(prompt)) > c.MaxPromptBytes {
		fail(w, http.StatusRequestEntityTooLarge, "prompt too large")
		return
	}
	ws := r.PathValue("w")
	at, err := s.placeOf(r.Context(), r.PathValue("t"), ws)
	if err != nil {
		fail(w, http.StatusConflict, "workspace is not reachable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	streamChat(ctx, w, "incus", at.argv(chatIncusArgs(ws, c.User, argv)), prompt)
}

// chatIncusArgs runs argv as the workspace user's login shell. "--" stops su parsing the CLI's
// flags; "$0" "$@" passes argv through without any shell re-parsing.
func chatIncusArgs(ws, user string, argv []string) []string {
	return append([]string{"exec", "--force-noninteractive", Instance(ws), "--", "su", "-", user, "-s", "/bin/bash",
		"-c", `exec "$0" "$@"`, "--"}, argv...)
}

// streamChat streams name's stdout to w as it comes, then the {"shippin_exit","stderr"} trailer.
func streamChat(ctx context.Context, w io.Writer, name string, args []string, stdin []byte) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var errb bytes.Buffer
	cmd.Stderr = &limited{b: &errb, n: 4096}
	fl, _ := w.(http.Flusher)
	cmd.Stdout = &flushWriter{w: w, f: fl}
	code := 0
	if err := cmd.Run(); err != nil {
		var x *exec.ExitError
		code = -1
		if errors.As(err, &x) {
			code = x.ExitCode()
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"shippin_exit": code, "stderr": errb.String()})
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if f.f != nil {
		f.f.Flush()
	}
	return n, err
}

type limited struct {
	b *bytes.Buffer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	if room := l.n - l.b.Len(); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		l.b.Write(p[:room])
	}
	return len(p), nil
}
