package hostd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// findArgs is find without the stdin recorded as the last element, so a pushed script
// body cannot satisfy an argv assertion.
func (f *fakeRunner) findArgs(needles ...string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		args := c
		if len(args) > 0 {
			args = args[:len(args)-1]
		}
		j := strings.Join(args, " ")
		ok := true
		for _, n := range needles {
			ok = ok && strings.Contains(j, n)
		}
		if ok {
			return args
		}
	}
	return nil
}

func sessionsServer() *Server {
	s := agentServer()
	s.cfg.Agent.SessionManager = true
	s.cfg.Agent.Providers["grok"] = AgentProvider{Run: []string{"grok"}, Resume: []string{"grok", "--resume"}}
	return s
}

func TestSessionManagerFlagOffIsNoop(t *testing.T) {
	s := agentServer()
	if s.cfg.Agent.sessions() {
		t.Fatal("default test agent must not boot the session manager")
	}
	f := useFake(t)
	if err := s.ensureSessions(context.Background(), place{t: tid}, "w-"+wsid); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("flag off touched incus: %v", f.calls)
	}
}

func TestSessionManagerRequiresSession(t *testing.T) {
	a := AgentConfig{SessionManager: true}
	if a.check() == nil {
		t.Fatal("session_manager without a session accepted")
	}
	a = sessionsServer().cfg.Agent
	if err := a.check(); err != nil {
		t.Fatal(err)
	}
	a.Providers["grok"] = AgentProvider{Run: []string{"grok", "--always-approve"}, Resume: []string{"grok", "--resume"}}
	if a.check() == nil {
		t.Fatal("--always-approve accepted")
	}
	a = sessionsServer().cfg.Agent
	a.Providers["codex"] = AgentProvider{Run: []string{"codex", "--dangerously-bypass-approvals-and-sandbox"}, Resume: []string{"codex"}}
	if a.check() == nil {
		t.Fatal("codex sandbox bypass accepted")
	}
}

func TestEnsureSessionsBootsProviders(t *testing.T) {
	s := sessionsServer()
	f := useFake(t)
	if err := s.ensureSessions(context.Background(), place{t: tid}, "w-"+wsid); err != nil {
		t.Fatal(err)
	}
	if f.find("systemctl enable --now shippin-sessions.service") == nil {
		t.Fatal("unit not enabled")
	}
	boot := f.findArgs("--user 1000", sessionsBin+" boot")
	if boot == nil {
		t.Fatalf("boot not run as the dev user: %v", f.calls)
	}
	prov := f.find("/etc/shippin/sessions/providers")
	if prov == nil {
		t.Fatal("providers file not written")
	}
	body := prov[len(prov)-1]
	if body != "claude\ncodex\ngrok\n" {
		t.Fatalf("providers %q", body)
	}
	if !strings.Contains(f.find("/etc/shippin/agent/grok.resume")[len(f.find("/etc/shippin/agent/grok.resume"))-1], "--resume") {
		t.Fatal("grok resume argv not installed")
	}
	unit := f.find("shippin-sessions.service")
	if unit == nil || !strings.Contains(unit[len(unit)-1], "User=dev") || !strings.Contains(unit[len(unit)-1], "ConditionPathExists=/etc/shippin/sessions/enabled") {
		t.Fatalf("unit: %v", unit)
	}
}

func TestStartProviderWindow(t *testing.T) {
	s := sessionsServer()
	f := useFake(t)
	started, err := s.StartAgent(context.Background(), tid, wsid, AgentReq{Provider: "grok", Resume: true})
	if err != nil || !started {
		t.Fatalf("start %v %v", started, err)
	}
	if f.findArgs("tmux respawn-window -k -t agent:grok", "/usr/local/bin/shippin-agent grok --resume") == nil {
		t.Fatal("grok was not resumed in its window")
	}
	if f.findArgs("tmux new-session") != nil {
		t.Fatal("session manager must not create a second tmux session for the CLI")
	}
	f = useFake(t, fakeReply{match: "display-message", out: "grok\n"})
	started, err = s.StartAgent(context.Background(), tid, wsid, AgentReq{Provider: "grok"})
	if err != nil || started || f.find("respawn-window") != nil {
		t.Fatalf("live grok window must be left alone: started=%v err=%v", started, err)
	}
	if _, err := s.StartAgent(context.Background(), tid, wsid, AgentReq{Provider: "bash"}); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestSessionsBootScript(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logf := filepath.Join(dir, "tmux.log")
	fake := "#!/bin/sh\necho \"$*\" >> " + logf + "\n[ \"$1\" = has-session ] && exit 1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	prov := filepath.Join(dir, "providers")
	if err := os.WriteFile(prov, []byte("claude\ncodex\ngrok\n;rm -rf /\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "shippin-sessions")
	if err := os.WriteFile(script, []byte(sessionsScript), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(script, "boot")
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "HOME="+home, "SESSION=agent", "SHIPPIN_SESSIONS_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("boot: %v %s", err, out)
	}
	got, err := os.ReadFile(logf)
	if err != nil {
		t.Fatal(err)
	}
	log := string(got)
	for _, want := range []string{"new-session -d -s agent -n shell", "new-window -d -t agent: -n claude", "new-window -d -t agent: -n codex", "new-window -d -t agent: -n grok"} {
		if !strings.Contains(log, want) {
			t.Errorf("log missing %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "rm -rf") || strings.Contains(log, ";rm") {
		t.Fatalf("rejected provider was created: %s", log)
	}
	st, err := os.ReadFile(filepath.Join(home, ".shippin", "sessions.json"))
	if err != nil || !strings.Contains(string(st), `"ready":true`) {
		t.Fatalf("status %q %v", st, err)
	}
	bad := exec.Command(script, "boot")
	bad.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "HOME="+home, "SESSION=Agent", "SHIPPIN_SESSIONS_DIR="+dir)
	if err := bad.Run(); err == nil {
		t.Fatal("bad session name accepted")
	}
}
