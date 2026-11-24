package hostd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrustBridgeCAOnCreate(t *testing.T) {
	f := useFake(t)
	s := testServer()
	if err := s.trustBridgeCA(context.Background(), place{t: "t1"}, "w-x", "x"); err != nil {
		t.Fatal("off must be a no-op:", err)
	}
	if len(f.calls) != 0 {
		t.Fatal("ran incus while off")
	}
	pub := filepath.Join(t.TempDir(), "ca.pub")
	_ = os.WriteFile(pub, []byte("ssh-ed25519 AAAATEST shippin-bridge-ca\n"), 0o644)
	s.cfg.BridgeCA = BridgeCAConfig{PublicKeyFile: pub, SSHUser: "dev", PrincipalPrefix: "w-"}
	if err := s.trustBridgeCA(context.Background(), place{t: "t1"}, "w-abc", "abc"); err != nil {
		t.Fatal(err)
	}
	c := strings.Join(f.calls[0], " ")
	for _, want := range []string{"--project=t-t1", "exec w-abc -- sh -c", "bridge-ca w-abc dev", "AuthorizedPrincipalsFile /etc/ssh/auth_principals/%%u", "ssh-ed25519 AAAATEST"} {
		if !strings.Contains(c, want) {
			t.Fatalf("missing %q in %s", want, c)
		}
	}
	s.cfg.BridgeCA.SSHUser = "bad user;rm"
	if s.trustBridgeCA(context.Background(), place{t: "t1"}, "w-abc", "abc") == nil {
		t.Fatal("unsafe user accepted")
	}
}
