package hostd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestHostGitleaksPinned(t *testing.T) {
	bin := []byte("\x7fELF fake gitleaks")
	sum := sha256.Sum256(bin)
	pin := GitleaksPin{Version: "8.28.0", SHA256: hex.EncodeToString(sum[:]), HostPath: "/usr/local/lib/shippin/gitleaks", Path: "/usr/local/lib/shippin/gitleaks"}
	if err := pin.check(); err != nil {
		t.Fatal(err)
	}
	if (GitleaksPin{Version: "8.28.0", SHA256: pin.SHA256, HostPath: "/usr/bin/gitleaks", Path: pin.Path}).check() == nil {
		t.Fatal("a pin outside /usr/local/lib/shippin must be rejected")
	}
	old := readHostFile
	t.Cleanup(func() { readHostFile = old })
	s := &Server{cfg: &Config{Git: GitConfig{Gitleaks: pin}}}
	readHostFile = func(string) ([]byte, error) { return bin, nil }
	if err := s.verifyHostGitleaks(); err != nil {
		t.Fatal(err)
	}
	readHostFile = func(string) ([]byte, error) { return []byte("tampered"), nil }
	if s.verifyHostGitleaks() == nil {
		t.Fatal("a host binary that does not match the pin must refuse")
	}
	readHostFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if s.verifyHostGitleaks() == nil {
		t.Fatal("missing host binary must refuse")
	}
	if (&Server{cfg: &Config{}}).verifyHostGitleaks() == nil {
		t.Fatal("no pin configured must refuse")
	}
	if s.hostRun(context.Background(), nil, nil, "sh", "-c", "true") == nil || s.hostRun(context.Background(), nil, nil, "/usr/bin/gitleaks", "dir") == nil {
		t.Fatal("host side may only run git, tar and the pinned gitleaks")
	}
}
