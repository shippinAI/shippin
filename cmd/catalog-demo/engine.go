package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// engines runs one real VS Code server (code-server) per demo workspace on loopback, standing in
// for the engine inside the workspace container. Each workspace keeps its own home and editor
// state directory, so stop + start resumes exactly like the home volume h-<uuid> does on Gold.
type engines struct {
	bin, root string
	mu        sync.Mutex
	procs     map[string]*exec.Cmd
}

func (e *engines) dir(ws string) string { return filepath.Join(e.root, ws) }

// port is stable per workspace (derived from its SSH port slot) so the engine URL survives restarts.
func enginePort(sshPort int) int { return 18000 + sshPort%1000 }

func (e *engines) start(ws, project string, port int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c := e.procs[ws]; c != nil && c.ProcessState == nil {
		return nil
	}
	home := filepath.Join(e.dir(ws), "home", project)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	if err := engineDefaults(filepath.Join(e.dir(ws), "state", "User")); err != nil {
		return err
	}
	c := exec.Command(e.bin, "--bind-addr", fmt.Sprintf("127.0.0.1:%d", port), "--auth", "none",
		"--user-data-dir", filepath.Join(e.dir(ws), "state"), "--extensions-dir", filepath.Join(e.dir(ws), "ext"),
		"--disable-telemetry", "--disable-update-check", home)
	logf, _ := os.Create(filepath.Join(e.dir(ws), "engine.log"))
	c.Stdout, c.Stderr = logf, logf
	if err := c.Start(); err != nil {
		return err
	}
	e.procs[ws] = c
	go func() { _ = c.Wait() }()
	for i := 0; i < 50; i++ {
		if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond); err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("engine did not start")
}

func (e *engines) stop(ws string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c := e.procs[ws]; c != nil && c.Process != nil {
		_ = c.Process.Signal(os.Interrupt)
		delete(e.procs, ws)
	}
}

func (e *engines) running(ws string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.procs[ws]
	return c != nil && c.ProcessState == nil
}

// engineDefaults: the engine runs behind Shippin's surfaces, so its own first-run prompts (trust,
// welcome page, parent-folder git) are off. Written once; later user changes are kept.
func engineDefaults(dir string) error {
	f := filepath.Join(dir, "settings.json")
	if _, err := os.Stat(f); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(`{"security.workspace.trust.enabled": false, "workbench.startupEditor": "none",
 "git.openRepositoryInParentFolders": "never", "chat.disableAIFeatures": true}`+"\n"), 0o600)
}
