package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// WriterConfig is what the in-workspace credential writer reads (cmd/shippin-credwriter). It
// carries argv and env names only, never a credential.
type WriterConfig struct {
	SpoolDir   string                    `json:"spool_dir"`
	StoreDir   string                    `json:"store_dir"`
	StatusPath string                    `json:"status_path"`
	RefreshSec int                       `json:"refresh_seconds"`
	Env        map[string]string         `json:"env"`
	Providers  map[string]WriterProvider `json:"providers"`
}

type WriterProvider struct {
	Status     []string `json:"status"`
	Refresh    []string `json:"refresh,omitempty"`
	TokenEnv   string   `json:"token_env"`
	TokenLogin []string `json:"token_login,omitempty"`
	Logout     []string `json:"logout"`
}

func (s *Server) writerConfig() WriterConfig {
	c := s.cfg.Connect
	wc := WriterConfig{SpoolDir: c.SpoolDir, StoreDir: c.StoreDir, StatusPath: c.StatusPath, RefreshSec: c.RefreshSeconds, Env: c.Env, Providers: map[string]WriterProvider{}}
	for id, p := range c.Providers {
		wc.Providers[id] = WriterProvider{Status: p.Status, Refresh: p.Refresh, TokenEnv: p.TokenEnv, TokenLogin: p.TokenLogin, Logout: p.Logout}
	}
	return wc
}

// writerUnit is the systemd unit of the one writer per workspace (Restart=always; it takes a
// lock in store_dir so a second copy exits). "+" lines run as root to make the /run dirs.
func (s *Server) writerUnit() string {
	a, c := s.cfg.AI, s.cfg.Connect
	u := fmt.Sprint(a.UID)
	return strings.Join([]string{"[Unit]", "Description=Shippin credential writer", "After=network-online.target", "",
		"[Service]", "User=" + u, "Group=" + u,
		"ExecStartPre=+/usr/bin/install -d -m 0700 -o " + u + " -g " + u + " " + c.SpoolDir + " " + path.Dir(c.StatusPath),
		"ExecStart=" + c.WriterPath + " -config " + path.Join(c.StoreDir, "writer.json"),
		"Restart=always", "RestartSec=5", "", "[Install]", "WantedBy=multi-user.target", ""}, "\n")
}

// kickWriter installs (idempotently) and starts the workspace's credential writer.
func (s *Server) kickWriter(ctx context.Context, at place, inst string) error {
	a, c := s.cfg.AI, s.cfg.Connect
	u := fmt.Sprint(a.UID)
	cfg, _ := json.Marshal(s.writerConfig())
	steps := [][]string{
		{"exec", inst, "--", "install", "-d", "-m", "0700", "-o", u, "-g", u, c.StoreDir, path.Dir(c.WriterPath)},
		{"file", "push", "--uid", u, "--gid", u, "--mode", "0755", c.WriterBin, inst + c.WriterPath},
	}
	for _, st := range steps {
		if _, err := at.do(ctx, st...); err != nil {
			return fmt.Errorf("credential writer not installed")
		}
	}
	if _, err := at.inc(ctx, string(cfg), "exec", inst, "--user", u, "--group", u, "--env", "F="+path.Join(c.StoreDir, "writer.json"), "--",
		"sh", "-c", `umask 077 && cat > "$F.tmp" && mv -f "$F.tmp" "$F"`); err != nil {
		return fmt.Errorf("credential writer not configured")
	}
	hook := "[ -r " + path.Join(c.StoreDir, "env") + " ] && . " + path.Join(c.StoreDir, "env") + "\n"
	for f, body := range map[string]string{"/etc/systemd/system/" + c.WriterUnit: s.writerUnit(), c.ProfilePath: hook} {
		if _, err := at.inc(ctx, body, "exec", inst, "--env", "F="+f, "--", "sh", "-c", `cat > "$F"`); err != nil {
			return fmt.Errorf("credential writer unit not written")
		}
	}
	for _, st := range [][]string{{"daemon-reload"}, {"enable", c.WriterUnit}, {"restart", c.WriterUnit}} {
		if _, err := at.do(ctx, append([]string{"exec", inst, "--", "systemctl"}, st...)...); err != nil {
			return fmt.Errorf("credential writer did not start")
		}
	}
	return nil
}

// spool drops one request file for the writer (stdin, mode 0600; never argv).
func (s *Server) spool(ctx context.Context, at place, inst, name, body string) error {
	a, c := s.cfg.AI, s.cfg.Connect
	u := fmt.Sprint(a.UID)
	if _, err := at.do(ctx, "exec", inst, "--", "install", "-d", "-m", "0700", "-o", u, "-g", u, c.SpoolDir); err != nil {
		return err
	}
	_, err := at.inc(ctx, body, "exec", inst, "--user", u, "--group", u, "--env", "F="+path.Join(c.SpoolDir, name), "--",
		"sh", "-c", `umask 077 && cat > "$F.tmp" && mv -f "$F.tmp" "$F"`)
	return err
}
