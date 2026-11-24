package ailaunch

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// wsDial is a minimal masked WebSocket client for the tests.
func wsDial(t *testing.T, srv *httptest.Server, path string) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	req := "GET " + path + " HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nOrigin: http://allowed.test\r\n" +
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
	_, _ = c.Write([]byte(req))
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	if !strings.Contains(line, "101") {
		t.Fatalf("handshake: %q", line)
	}
	for l, _ := br.ReadString('\n'); l != "\r\n"; l, _ = br.ReadString('\n') {
	}
	return c, br
}

func send(c net.Conn, op byte, p []byte) {
	h := []byte{0x80 | op, 0x80 | byte(len(p)), 1, 2, 3, 4}
	m := make([]byte, len(p))
	for i := range p {
		m[i] = p[i] ^ h[2+i%4]
	}
	_, _ = c.Write(append(h, m...))
}

// readUntil collects server frames until want shows up or the deadline passes.
func readUntil(c net.Conn, br *bufio.Reader, want string, d time.Duration) string {
	var all bytes.Buffer
	_ = c.SetReadDeadline(time.Now().Add(d))
	for !strings.Contains(all.String(), want) {
		var h [2]byte
		if _, err := br.Read(h[:1]); err != nil {
			break
		}
		if _, err := br.Read(h[1:]); err != nil {
			break
		}
		n := int(h[1] & 0x7f)
		if n == 126 {
			var b [2]byte
			_, _ = br.Read(b[:1])
			_, _ = br.Read(b[1:])
			n = int(binary.BigEndian.Uint16(b[:]))
		}
		p := make([]byte, n)
		if _, err := ioFull(br, p); err != nil {
			break
		}
		all.Write(p)
	}
	return all.String()
}

func ioFull(br *bufio.Reader, p []byte) (int, error) {
	n := 0
	for n < len(p) {
		m, err := br.Read(p[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func TestSessionSurvivesReconnect(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	cfg := &Config{TmuxSocket: "ailaunch-test", SessionPrefix: "t-", Origins: []string{"http://allowed.test"},
		CLIs: []CLI{{ID: "fake", Label: "Fake CLI", Argv: []string{"sh", "-c", "echo FAKE-CLI-READY; exec cat"}}}}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", cfg.TmuxSocket, "kill-server").Run() })
	mux := http.NewServeMux()
	(&Handler{Cfg: cfg}).Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	r, _ := http.Post(srv.URL+"/v1/ai/sessions/fake?rows=24&cols=80", "", nil)
	if r.StatusCode != 200 {
		t.Fatalf("launch: %d", r.StatusCode)
	}
	before := cfg.Status(t.Context(), &cfg.CLIs[0])
	c, br := wsDial(t, srv, "/v1/ai/sessions/fake/attach?rows=24&cols=80")
	if out := readUntil(c, br, "FAKE-CLI-READY", 5*time.Second); !strings.Contains(out, "FAKE-CLI-READY") {
		t.Fatalf("no CLI output on first attach: %q", out)
	}
	send(c, 0x2, []byte("marker-before-drop\r"))
	readUntil(c, br, "marker-before-drop", 3*time.Second)
	c.Close() // drop the socket mid-session
	time.Sleep(300 * time.Millisecond)
	if s := cfg.Status(t.Context(), &cfg.CLIs[0]); !s.Running || s.PanePID != before.PanePID || s.Created != before.Created {
		t.Fatalf("session did not survive the disconnect: before %+v after %+v", before, s)
	}
	c2, br2 := wsDial(t, srv, "/v1/ai/sessions/fake/attach?rows=24&cols=80")
	defer c2.Close()
	if out := readUntil(c2, br2, "marker-before-drop", 5*time.Second); !strings.Contains(out, "marker-before-drop") {
		t.Fatalf("reconnect did not resume the screen: %q", out)
	}
	if r, _ := http.Post(srv.URL+"/v1/ai/sessions/fake", "", nil); r.StatusCode != 200 {
		t.Fatal("second launch should be idempotent")
	}
	if s := cfg.Status(t.Context(), &cfg.CLIs[0]); s.PanePID != before.PanePID {
		t.Fatal("second click started a new CLI instead of reusing the session")
	}
}
