package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// When ssh runs this test binary as `shippin-bridge` (through the wrapper below), act as main.
func TestMain(m *testing.M) {
	if os.Getenv("SHIPPIN_BRIDGE_TEST_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func needOpenSSH(t *testing.T) string {
	bin, err := exec.LookPath("ssh")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("needs the OpenSSH client on a Unix host")
	}
	return bin
}

// The block parses in real OpenSSH and resolves to the bridge's files, for paths with spaces too.
func TestConfigBlockIsWhatOpenSSHReads(t *testing.T) {
	sshBin := needOpenSSH(t)
	dir := filepath.Join(t.TempDir(), "the operator H", ".shippin", "bridge")
	cfg := filepath.Join(t.TempDir(), "ssh_config")
	block := configBlock(workspace{ID: "w1", Name: "dev"}, "dev", "/bin/true", dir)
	if err := os.WriteFile(cfg, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(sshBin, "-F", cfg, "-G", "shippin-dev").CombinedOutput()
	if err != nil {
		t.Fatalf("ssh -G: %v\n%s", err, out)
	}
	for _, want := range []string{
		"user dev", "hostname w1", "proxycommand /bin/true proxy w1", "identitiesonly yes",
		"identityfile " + filepath.Join(dir, "ssh", "w1"), "certificatefile " + filepath.Join(dir, "ssh", "w1-cert.pub"),
		"hostkeyalias shippin-dev", "stricthostkeychecking accept-new",
	} {
		if !strings.Contains(string(out), want+"\n") {
			t.Errorf("ssh -G lacks %q:\n%s", want, out)
		}
	}
	other, _ := exec.Command(sshBin, "-F", cfg, "-G", "somewhere-else").CombinedOutput()
	if strings.Contains(string(other), "proxy w1") {
		t.Error("the block applied to another host")
	}
}

// Real OpenSSH reaches a workspace through the bridge: Match exec signs a certificate, ProxyCommand
// relays over TLS, the sshd accepts only the CA-signed certificate, and a second run reuses it.
func TestOpenSSHReachesAWorkspaceThroughTheBridge(t *testing.T) {
	sshBin := needOpenSSH(t)
	w := newWorld(t)
	home, bin := t.TempDir(), t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wrapper := "#!/bin/sh\nSHIPPIN_BRIDGE_TEST_MAIN=1 exec \"" + self + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "shippin-bridge"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".shippin", "bridge")
	cfg := filepath.Join(home, "ssh_config")
	if err := os.WriteFile(cfg, []byte(configBlock(workspace{ID: "w1", Name: "dev"}, "dev", "shippin-bridge", dir)), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SHIPPIN_BRIDGE_TOKEN=tok", "SHIPPIN_URL="+w.url, "SSL_CERT_FILE="+w.caFile)
	for run := 1; run <= 2; run++ {
		cmd := exec.Command(sshBin, "-F", cfg, "-o", "BatchMode=yes", "shippin-dev", "echo hi")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "ran: echo hi") {
			t.Fatalf("run %d: %v\n%s", run, err, out)
		}
	}
	if n := w.certs.Load(); n != 1 {
		t.Errorf("certificates signed: %d, want 1 (the second connection reuses the live one)", n)
	}
	if info, err := os.Stat(filepath.Join(dir, "ssh", "w1")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("private key file: %v %v", info, err)
	}
	if known, _ := os.ReadFile(filepath.Join(dir, "known_hosts")); !strings.Contains(string(known), "shippin-dev") {
		t.Errorf("host key not pinned under the alias: %q", known)
	}

	// Signed out of the bridge: Match exec fails, the block doesn't apply, and ssh says so.
	cmd := exec.Command(sshBin, "-F", cfg, "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "shippin-dev", "true")
	cmd.Env = append(env, "SHIPPIN_BRIDGE_TOKEN=wrong")
	_ = os.Remove(filepath.Join(dir, "ssh", "w1-cert.pub"))
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("a wrong bridge token still connected:\n%s", out)
	}
}
