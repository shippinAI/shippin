//go:build !windows

// shippin-credwriter is the one credential writer per workspace ("activate once, resume
// forever"): systemd keeps it running, a lock in store_dir keeps it single, and its store lives
// on the AI home so it resumes after every restart.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"shippin.cloud/console/internal/credwriter"
	"shippin.cloud/console/internal/hostd"
)

func main() {
	path := flag.String("config", "", "writer.json written by shippin-hostd")
	flag.Parse()
	raw, err := os.ReadFile(*path)
	var cfg hostd.WriterConfig
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.RefreshSec < 1 {
		log.Fatal("credwriter: config unreadable")
	}
	lock, err := os.OpenFile(filepath.Join(cfg.StoreDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil || syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		log.Fatal("credwriter: another writer holds the lock")
	}
	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	w := &credwriter.Writer{Cfg: cfg, Now: time.Now, Run: func(argv []string, stdin string) bool {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env, cmd.Stdin = env, strings.NewReader(stdin)
		var sink bytes.Buffer // vendor output may carry credentials: discarded, never logged
		cmd.Stdout, cmd.Stderr = &sink, &sink
		return cmd.Run() == nil
	}}
	// hostd restarts the unit after every spooled change, so each change is applied at once.
	for {
		w.Tick(true)
		time.Sleep(time.Duration(cfg.RefreshSec) * time.Second)
	}
}
