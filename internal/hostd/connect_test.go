package hostd

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const cwid = "019a0000-0000-7000-8000-0000000000c3"

func connectServer() *Server {
	s := aiServer()
	s.cfg.Connect = ConnectConfig{Env: map[string]string{"CLAUDE_CONFIG_DIR": "/home/dev/.ai/claude", "GROK_HOME": "/home/dev/.ai/grok"},
		SpoolDir: "/run/shippin/connect/in", StoreDir: "/home/dev/.ai/connect", StatusPath: "/run/shippin/connect/status.json",
		WriterUnit: "shippin-credwriter.service", WriterBin: "/opt/shippin/shippin-credwriter", WriterPath: "/home/dev/.ai/bin/shippin-credwriter",
		LoginSeconds: 60, RefreshSeconds: 60, ProfilePath: "/etc/profile.d/shippin-connect.sh", OutputMax: 4096, StatusMax: 1024,
		Providers: map[string]ConnectProvider{"github": {Login: []string{"gh", "auth", "login", "--web"}, Logout: []string{"gh", "auth", "logout"},
			Status: []string{"gh", "auth", "status"}, TokenEnv: "GH_TOKEN", TokenPattern: `^gh[pousr]_[A-Za-z0-9_]{20,255}$`,
			URLPrefixes: []string{"https://github.com/login/device"}, CodePattern: `code: ([A-Z0-9]{4}-[A-Z0-9]{4})`, PressEnter: true}}}
	return s
}

func TestConnectConfigCheck(t *testing.T) {
	if err := connectServer().cfg.Connect.check(); err != nil {
		t.Fatal(err)
	}
	for _, mut := range []func(*ConnectConfig){
		func(c *ConnectConfig) { c.SpoolDir = "/tmp/x" },
		func(c *ConnectConfig) { c.Env = map[string]string{"bad name": "x"} },
		func(c *ConnectConfig) {
			p := c.Providers["github"]
			p.URLPrefixes = []string{"http://x"}
			c.Providers["github"] = p
		},
	} {
		c := connectServer().cfg.Connect
		mut(&c)
		if c.check() == nil {
			t.Fatalf("must fail: %+v", c)
		}
	}
}

func TestPutTokenNeverOnArgv(t *testing.T) {
	s := connectServer()
	f := useFake(t)
	tok := "ghp_" + strings.Repeat("a", 36)
	if err := s.PutToken(context.Background(), tid, cwid, "github", SpoolReq{Token: tok}); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c[:len(c)-1], " "), tok) {
			t.Fatalf("token on argv: %v", c[:len(c)-1])
		}
	}
	if f.find("github.json", tok) == nil || f.find("systemctl restart shippin-credwriter.service") == nil {
		t.Fatal("token not spooled on stdin or writer not kicked")
	}
	if s.PutToken(context.Background(), tid, cwid, "github", SpoolReq{Token: "nope"}) == nil {
		t.Fatal("bad token accepted")
	}
	if s.PutToken(context.Background(), tid, cwid, "nobody", SpoolReq{Token: tok}) == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestStartLoginSurfacesCodeOnly(t *testing.T) {
	s := connectServer()
	useFake(t)
	old := streamCmd
	streamCmd = func(ctx context.Context, _ place, args []string) *exec.Cmd {
		if !strings.Contains(strings.Join(args, " "), "CLAUDE_CONFIG_DIR=/home/dev/.ai/claude") {
			t.Error("CLI homes not set")
		}
		return exec.CommandContext(ctx, "sh", "-c", `read x; echo "one-time code: WXYZ-1234 secret gho_x"; echo https://github.com/login/device; sleep 1`)
	}
	t.Cleanup(func() { streamCmd = old })
	if _, err := s.StartLogin(context.Background(), tid, cwid, "github"); err != nil {
		t.Fatal(err)
	}
	var st *LoginState
	for i := 0; i < 50; i++ {
		if st, _ = s.Login(cwid, "github"); st != nil && st.State == "waiting" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st == nil || st.Code != "WXYZ-1234" || st.URL != "https://github.com/login/device" {
		t.Fatalf("state %+v", st)
	}
	for i := 0; i < 100 && st.State != "connected"; i++ {
		time.Sleep(30 * time.Millisecond)
		st, _ = s.Login(cwid, "github")
	}
	if st.State != "connected" || st.Code != "" {
		t.Fatalf("final %+v", st)
	}
}

func TestShippedConnectConfig(t *testing.T) {
	cfg, err := LoadConfig("../../deploy/hostd.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Connect.check(); err != nil || len(cfg.Connect.Providers) != 4 {
		t.Fatalf("shipped connect section: %v", err)
	}
	u, c := loginPrompt(cfg.Connect.Providers["codex"], "1. Open\n   https://auth.openai.com/codex/device\n2. Enter this one-time code\n   6PA9-8UKNY\n")
	if u != "https://auth.openai.com/codex/device" || c != "6PA9-8UKNY" {
		t.Fatalf("codex prompt %q %q", u, c)
	}
	u, c = loginPrompt(cfg.Connect.Providers["claude"], "Browser didn't open? https://claude.com/cai/oauth/authorize?code=true&x=1\nPaste code here")
	if u == "" || c != "" {
		t.Fatalf("claude prompt %q %q", u, c)
	}
}
