package credwriter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/hostd"
)

func TestTickPersistsAndForgets(t *testing.T) {
	d := t.TempDir()
	cfg := hostd.WriterConfig{SpoolDir: filepath.Join(d, "in"), StoreDir: filepath.Join(d, "store"), StatusPath: filepath.Join(d, "status.json"),
		Env: map[string]string{"GROK_HOME": "/home/dev/.ai/grok"},
		Providers: map[string]hostd.WriterProvider{
			"github": {Status: []string{"gh", "auth", "status"}, TokenEnv: "GH_TOKEN", TokenLogin: []string{"gh", "auth", "login", "--with-token"}, Logout: []string{"gh", "auth", "logout"}},
			"grok":   {Status: []string{"grok-ok"}, TokenEnv: "XAI_API_KEY", Logout: []string{"grok", "logout"}}}}
	_ = os.MkdirAll(cfg.SpoolDir, 0o700)
	_ = os.MkdirAll(cfg.StoreDir, 0o700)
	var ran []string
	w := &Writer{Cfg: cfg, Now: time.Now, Run: func(argv []string, stdin string) bool {
		if strings.Contains(strings.Join(argv, " "), "ghp_") {
			t.Fatal("token on argv")
		}
		ran = append(ran, strings.Join(argv, " ")+"|"+stdin)
		return argv[0] == "grok-ok"
	}}
	_ = os.WriteFile(filepath.Join(cfg.SpoolDir, "github.json"), []byte(`{"op":"set","token":"ghp_abc"}`), 0o600)
	w.Tick(true)
	env, _ := os.ReadFile(filepath.Join(cfg.StoreDir, "env"))
	if !strings.Contains(string(env), "export GH_TOKEN='ghp_abc'") || !strings.Contains(string(env), "GROK_HOME=") {
		t.Fatalf("env %q", env)
	}
	if ran[0] != "gh auth login --with-token|ghp_abc\n" {
		t.Fatalf("token login %v", ran)
	}
	var st hostd.ConnectStatus
	b, _ := os.ReadFile(cfg.StatusPath)
	_ = json.Unmarshal(b, &st)
	if !st.Providers[0].Connected || st.Providers[0].Method != "token" || !st.Providers[1].Connected || st.Providers[1].Method != "browser" {
		t.Fatalf("status %+v", st)
	}
	if strings.Contains(string(b), "ghp_") {
		t.Fatal("status leaks token")
	}
	// restart: a new writer resumes from the store; then forget removes it.
	w2 := &Writer{Cfg: cfg, Now: time.Now, Run: w.Run}
	_ = os.WriteFile(filepath.Join(cfg.SpoolDir, "github.json"), []byte(`{"op":"forget"}`), 0o600)
	w2.Tick(false)
	env, _ = os.ReadFile(filepath.Join(cfg.StoreDir, "env"))
	if strings.Contains(string(env), "GH_TOKEN") || !slices.Contains(ran, "gh auth logout|") {
		t.Fatalf("forget: %q %v", env, ran)
	}
}
