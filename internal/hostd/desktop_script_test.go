package hostd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"shippin.cloud/console/internal/apps"
)

func TestPrereqScriptDoesNotInstallApps(t *testing.T) {
	b, err := os.ReadFile("desktop_prereq.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, bad := range []string{"claude-desktop", "claude-code", "chatgpt.com", "ANTHROPIC_API_KEY", "codex/install.sh"} {
		if strings.Contains(s, bad) {
			t.Errorf("prereq script contains %q", bad)
		}
	}
}

func TestDesktopPrereqAdoptDoesNotRestart(t *testing.T) {
	root := t.TempDir()
	writeJesseFixture(t, root, "127.0.0.1", "false")
	marker := "CLAUDE_LIST_MARKER stay"
	writeFixture(t, filepath.Join(root, "etc/apt/sources.list.d/claude-desktop.list"), marker+"\n")
	log := filepath.Join(root, "stub.log")
	runPrereq(t, root, log, "0")
	runPrereq(t, root, log, "0")
	if n := strings.Count(readFile(t, log), "try-restart"); n != 0 {
		t.Fatalf("try-restart count %d\n%s", n, readFile(t, log))
	}
	if strings.Contains(readFile(t, log), "usermod") {
		t.Fatal("usermod ran")
	}
	if got := readFile(t, filepath.Join(root, "etc/apt/sources.list.d/claude-desktop.list")); got != marker+"\n" {
		t.Fatalf("claude apt list changed: %q", got)
	}
	pam := readFile(t, filepath.Join(root, "etc/pam.d/xrdp-sesman"))
	if strings.Count(pam, "auth optional pam_gnome_keyring.so\n") != 1 || strings.Count(pam, "session optional pam_gnome_keyring.so auto_start\n") != 1 {
		t.Fatalf("pam lines duplicated:\n%s", pam)
	}
	ini := readFile(t, filepath.Join(root, "etc/xrdp/xrdp.ini"))
	if !strings.Contains(ini, "address=127.0.0.1") || strings.Contains(ini, "0.0.0.0") {
		t.Fatal(ini)
	}
}

func TestDesktopPrereqFixesPublicBindOnce(t *testing.T) {
	root := t.TempDir()
	writeJesseFixture(t, root, "0.0.0.0", "true")
	log := filepath.Join(root, "stub.log")
	runPrereq(t, root, log, "0")
	if n := strings.Count(readFile(t, log), "try-restart"); n != 1 {
		t.Fatalf("try-restart count %d\n%s", n, readFile(t, log))
	}
	ini := readFile(t, filepath.Join(root, "etc/xrdp/xrdp.ini"))
	ses := readFile(t, filepath.Join(root, "etc/xrdp/sesman.ini"))
	if !strings.Contains(ini, "address=127.0.0.1") || strings.Contains(ini, "address=0.0.0.0") {
		t.Fatal(ini)
	}
	if !strings.Contains(ses, "KillDisconnected=false") {
		t.Fatal(ses)
	}
	runPrereq(t, root, log, "0")
	if n := strings.Count(readFile(t, log), "try-restart"); n != 1 {
		t.Fatalf("second run restarted again: %d\n%s", n, readFile(t, log))
	}
}

func TestDesktopPrereqFreshTree(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(root, "stub.log")
	runPrereq(t, root, log, "1")
	ini := readFile(t, filepath.Join(root, "etc/xrdp/xrdp.ini"))
	ses := readFile(t, filepath.Join(root, "etc/xrdp/sesman.ini"))
	if !strings.Contains(ini, "address=127.0.0.1") || !strings.Contains(ini, "port=3389") {
		t.Fatal(ini)
	}
	if !strings.Contains(ses, "KillDisconnected=false") || !strings.Contains(ses, "Policy=Default") {
		t.Fatal(ses)
	}
	pam := readFile(t, filepath.Join(root, "etc/pam.d/xrdp-sesman"))
	if !strings.Contains(pam, "pam_gnome_keyring.so") {
		t.Fatal(pam)
	}
	if _, err := os.Stat(filepath.Join(root, "var/lib/shippin/desktop-ready")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, log), "try-restart") {
		t.Fatal("fresh inactive xrdp should be enabled, not restarted")
	}
	if !strings.Contains(readFile(t, log), "enable --now") {
		t.Fatal(readFile(t, log))
	}
}

func TestDesktopStatusIgnoresCredentialFiles(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "etc/passwd"), "root:x:0:0:root:/root:/bin/bash\noperator:x:10000:10000::/home/operator:/bin/bash\nevil:x:1:1::/home/evil:/bin/bash\n")
	writeFixture(t, filepath.Join(root, "etc/xrdp/xrdp.ini"), "[Globals]\naddress=127.0.0.1\nport=3389\n")
	writeFixture(t, filepath.Join(root, "home/operator/.claude.json"), "SECRETTOKEN\n")
	writeFixture(t, filepath.Join(root, "home/operator/.claude/.credentials"), "SECRETTOKEN\n")
	writeFixture(t, filepath.Join(root, "usr/share/applications/grok.desktop"), "SECRETTOKEN\n")
	bin := filepath.Join(root, "usr/bin/claude")
	writeFixture(t, bin, "#!/bin/sh\n")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "stub.log")
	writeStatusStubs(t, root, log)
	cmd := exec.Command("bash", "desktop_status.sh")
	cmd.Env = []string{"SHIPPIN_ROOT=" + root, "PATH=" + filepath.Join(root, "bin") + ":/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(string(out), "SECRETTOKEN") {
		t.Fatalf("status leaked a credential file:\n%s", out)
	}
	src, err := os.ReadFile("desktop_status.sh")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "sudo") || strings.Contains(string(src), ".claude") || strings.Contains(string(src), "credentials") {
		t.Fatal("status probe must ignore credential files and must not use sudo")
	}
	var st apps.Status
	if json.Unmarshal(out, &st) != nil {
		t.Fatalf("not json: %s", out)
	}
	apps.Normalize(&st)
	if st.SignedIn != "unknown" || st.Packages["claude-desktop"] != "3.6556.1" || !st.XRDPLoopback || !st.GrokDesktop {
		t.Fatalf("%+v", st)
	}
	if len(st.Users) != 1 || st.Users[0].User != "operator" || !st.Users[0].Claude {
		t.Fatalf("users %+v", st.Users)
	}
}

func writeJesseFixture(t *testing.T, root, address, kill string) {
	t.Helper()
	writeFixture(t, filepath.Join(root, "etc/xrdp/xrdp.ini"), "[Globals]\naddress="+address+"\nport=3389\n")
	writeFixture(t, filepath.Join(root, "etc/xrdp/sesman.ini"), "[Sessions]\nKillDisconnected="+kill+"\nDisconnectedTimeLimit=0\nIdleTimeLimit=0\nPolicy=Default\n")
	writeFixture(t, filepath.Join(root, "etc/pam.d/xrdp-sesman"), "auth optional pam_gnome_keyring.so\nsession optional pam_gnome_keyring.so auto_start\n")
	writeFixture(t, filepath.Join(root, "etc/passwd"), "operator:x:10000:10000::/home/operator:/bin/bash\n")
}

func runPrereq(t *testing.T, root, log, activeExit string) {
	t.Helper()
	writePrereqStubs(t, root, log, activeExit)
	cmd := exec.Command("bash", "desktop_prereq.sh")
	cmd.Env = []string{"SHIPPIN_ROOT=" + root, "PATH=" + filepath.Join(root, "bin") + ":/usr/bin:/bin", "DEBIAN_FRONTEND=noninteractive"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prereq: %v\n%s", err, out)
	}
}

func writePrereqStubs(t *testing.T, root, log, activeExit string) {
	t.Helper()
	dir := filepath.Join(root, "bin")
	stub(t, dir, "apt-get", "#!/bin/sh\necho \"apt-get $*\" >> \"$LOG\"\nexit 0\n")
	stub(t, dir, "systemctl", "#!/bin/sh\necho \"systemctl $*\" >> \"$LOG\"\nif [ \"$1\" = is-active ]; then exit \"$ACTIVE_EXIT\"; fi\nexit 0\n")
	stub(t, dir, "getent", "#!/bin/sh\necho \"getent $*\" >> \"$LOG\"\nexit 1\n")
	stub(t, dir, "usermod", "#!/bin/sh\necho \"usermod $*\" >> \"$LOG\"\nexit 0\n")
	stub(t, dir, "modprobe", "#!/bin/sh\necho \"modprobe $*\" >> \"$LOG\"\nexit 0\n")
	// The stubs read LOG and ACTIVE_EXIT from the environment. The prereq script does not
	// pass them unless they are exported into the command env, so bake the paths in.
	for _, name := range []string{"apt-get", "systemctl", "getent", "usermod", "modprobe"} {
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ReplaceAll(string(b), "$LOG", log)
		s = strings.ReplaceAll(s, "$ACTIVE_EXIT", activeExit)
		if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeStatusStubs(t *testing.T, root, log string) {
	t.Helper()
	dir := filepath.Join(root, "bin")
	stub(t, dir, "dpkg-query", "#!/bin/sh\npkg=\nfor a in \"$@\"; do pkg=$a; done\nif [ \"$pkg\" = claude-desktop ]; then echo 'claude-desktop 3.6556.1'; fi\nexit 0\n")
	stub(t, dir, "ss", "#!/bin/sh\necho 'LISTEN 0 128 127.0.0.1:3389 0.0.0.0:*'\nexit 0\n")
	_ = log
}

func stub(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
