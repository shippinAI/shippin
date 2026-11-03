package api

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/config"
)

func TestWSFrameRoundTrip(t *testing.T) {
	var wire bytes.Buffer
	p := []byte(strings.Repeat("ls -la\r", 40))
	mask := [4]byte{1, 2, 3, 4}
	wire.Write([]byte{0x82, 0x80 | 126, byte(len(p) >> 8), byte(len(p))})
	wire.Write(mask[:])
	for i, b := range p {
		wire.WriteByte(b ^ mask[i%4])
	}
	var out bytes.Buffer
	got, err := wsRead(bufio.NewReader(&wire), &out)
	if err != nil || !bytes.Equal(got, p) {
		t.Fatalf("read %q %v", got, err)
	}
	if err := wsWrite(&out, 0x2, []byte("hi")); err != nil || !bytes.Equal(out.Bytes(), []byte{0x82, 2, 'h', 'i'}) {
		t.Fatalf("write % x", out.Bytes())
	}
	unmasked := bytes.NewReader([]byte{0x82, 0x01, 'x'})
	if _, err := wsRead(bufio.NewReader(unmasked), &out); err == nil {
		t.Fatal("unmasked client frame accepted")
	}
}

func TestExpiryStatus(t *testing.T) {
	soon, later := time.Now().Add(time.Hour), time.Now().Add(30*24*time.Hour)
	if expiryStatus(&soon) != "expiring" || expiryStatus(&later) != "connected" || expiryStatus(nil) != "connected" {
		t.Fatal("expiry status")
	}
}

func TestConnectConfigAndIDELink(t *testing.T) {
	c := config.Connect{Enabled: true, VaultixEnv: "prod", VaultixPath: "/connect/{workspace}", IDEFolder: "/home/dev/code",
		Providers: []config.ConnectProvider{{ID: "github", Label: "GitHub", VaultixKey: "GITHUB_TOKEN"}}}
	if err := c.Check(); err != nil || c.Path("w1") != "/connect/w1" {
		t.Fatal(err)
	}
	bad := c
	bad.VaultixPath = "/connect"
	if bad.Check() == nil {
		t.Fatal("path without {workspace} accepted")
	}
	a := &API{cfg: &config.Config{Connect: c}}
	u := a.ideTerminalURL("https://dev-x.ws.example")
	if !strings.HasPrefix(u, "/auth/ws-login?rd=") || !strings.Contains(u, "folder%3D%252Fhome%252Fdev%252Fcode") {
		t.Fatalf("ide link %s", u)
	}
}
