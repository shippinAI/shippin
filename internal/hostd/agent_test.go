package hostd

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const wsid = "019a0000-0000-7000-8000-0000000000c3"

func agentServer() *Server {
	s := testServer()
	s.cfg.Agent = AgentConfig{User: "dev", UID: 1000, Session: "agent", PromptMaxBytes: 8192, TermPort: 7681,
		TermIface: "eth0", TermBase: "/_term", Providers: map[string]AgentProvider{
			"claude": {Run: []string{"claude"}, Resume: []string{"claude", "--continue"}},
			"codex":  {Run: []string{"codex"}, Resume: []string{"codex", "resume", "--last"}}}}
	return s
}

func TestAgentConfigCheck(t *testing.T) {
	if err := agentServer().cfg.Agent.check(); err != nil {
		t.Fatal(err)
	}
	for _, mut := range []func(*AgentConfig){
		func(a *AgentConfig) {
			a.Providers["claude"] = AgentProvider{Run: []string{"claude", "--dangerously-skip-permissions"}, Resume: []string{"x"}}
		},
		func(a *AgentConfig) {
			a.Providers["grok"] = AgentProvider{Run: []string{"grok", "--yolo"}, Resume: []string{"grok"}}
		},
		func(a *AgentConfig) {
			a.Providers["codex"] = AgentProvider{Run: []string{"codex; rm -rf ~"}, Resume: []string{"x"}}
		},
		func(a *AgentConfig) { a.TermBase = "/term/../x" },
		func(a *AgentConfig) { a.PromptMaxBytes = 1 << 20 },
	} {
		a := agentServer().cfg.Agent
		mut(&a)
		if a.check() == nil {
			t.Fatalf("must fail: %+v", a)
		}
	}
}

// The prompt must only ever travel on stdin, never in an argv hostd runs.
func TestPromptNeverInArgv(t *testing.T) {
	s := agentServer()
	f := useFake(t, fakeReply{match: "has-session", err: os.ErrNotExist})
	evil := `"; rm -rf ~ $(id) ` + "`whoami`"
	if err := s.PutPrompt(context.Background(), tid, wsid, evil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartAgent(context.Background(), tid, wsid, AgentReq{Provider: "claude"}); err != nil {
		t.Fatal(err)
	}
	wrote := false
	for _, c := range f.calls {
		argv := strings.Join(c[:len(c)-1], " ")
		if strings.Contains(argv, "rm -rf ~") || strings.Contains(argv, "whoami") {
			t.Fatalf("prompt leaked into argv: %v", c)
		}
		wrote = wrote || (c[len(c)-1] == evil && strings.Contains(argv, "--user 1000"))
	}
	if !wrote {
		t.Fatal("prompt not written as the user via stdin")
	}
	if f.find("tmux new-session -d -s agent", "/usr/local/bin/shippin-agent claude") == nil {
		t.Fatal("agent not started in tmux")
	}
	if f.find("tmux new-session", "--resume") != nil {
		t.Fatal("new run must not resume")
	}
}

func TestAgentProviderEnumAndLimits(t *testing.T) {
	s := agentServer()
	useFake(t)
	if _, err := s.StartAgent(context.Background(), tid, wsid, AgentReq{Provider: "bash -c id"}); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if s.PutPrompt(context.Background(), tid, wsid, strings.Repeat("x", 8193)) == nil || s.PutPrompt(context.Background(), tid, wsid, "  ") == nil {
		t.Fatal("prompt size limits")
	}
	f := useFake(t, fakeReply{match: "has-session"})
	started, err := s.StartAgent(context.Background(), tid, wsid, AgentReq{Provider: "codex", Resume: true})
	if err != nil || started || f.find("tmux new-session") != nil {
		t.Fatalf("existing session must be left alone: %v %v", started, err)
	}
}

func TestAgentStatusTyped(t *testing.T) {
	s := agentServer()
	useFake(t, fakeReply{match: "bash -c", out: `{"session":true,"activity":1760000000,"exited":false,"exit_code":-1,"files_changed":12}`})
	st, err := s.ReadAgentStatus(context.Background(), tid, wsid)
	if err != nil || !st.Session || st.FilesChanged != 12 {
		t.Fatalf("%+v %v", st, err)
	}
	useFake(t, fakeReply{match: "bash -c", out: "<html>"})
	if _, err := s.ReadAgentStatus(context.Background(), tid, wsid); err == nil {
		t.Fatal("malformed status accepted")
	}
}

func TestRouteHasPrivateTermPath(t *testing.T) {
	s := agentServer()
	b, _ := json.Marshal(s.route(wsid, []string{"a.ws.example.test"}, "10.128.9.5"))
	j := string(b)
	if !strings.Contains(j, `"path":["/_term/*"]`) || !strings.Contains(j, "10.128.9.5:7681") {
		t.Fatalf("term route missing: %s", j)
	}
	if strings.Count(j, "/auth/ws-check") != 2 {
		t.Fatal("term and IDE must both sit behind forward_auth")
	}
	s.cfg.Agent.TermPort = 0
	b, _ = json.Marshal(s.route(wsid, []string{"a.ws.example.test"}, "10.128.9.5"))
	if strings.Contains(string(b), "/_term") {
		t.Fatal("term route without a term port")
	}
}
