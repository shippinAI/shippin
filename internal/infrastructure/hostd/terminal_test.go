package hostd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTermFrameRoundTrip(t *testing.T) {
	var b bytes.Buffer
	big := bytes.Repeat([]byte("k"), 70000) // two data frames
	if err := WriteTermData(&b, big); err != nil {
		t.Fatal(err)
	}
	if err := WriteTermResize(&b, 40, 120); err != nil {
		t.Fatal(err)
	}
	var got []byte
	for range 2 {
		f, err := ReadTermFrame(&b)
		if err != nil || f.Kind != TermData {
			t.Fatalf("data frame: %+v %v", f, err)
		}
		got = append(got, f.Data...)
	}
	if !bytes.Equal(got, big) {
		t.Fatalf("data split lost bytes: %d", len(got))
	}
	f, err := ReadTermFrame(&b)
	if err != nil || f.Kind != TermResize || f.Rows != 40 || f.Cols != 120 {
		t.Fatalf("resize frame: %+v %v", f, err)
	}
	if _, err := ReadTermFrame(&b); err != io.EOF {
		t.Fatalf("clean end must be io.EOF, got %v", err)
	}
}

func TestTermFrameRefusals(t *testing.T) {
	if WriteTermResize(io.Discard, 0, 80) == nil || WriteTermResize(io.Discard, 24, TermMaxCols+1) == nil {
		t.Fatal("out-of-bounds resize must not be sent")
	}
	for name, raw := range map[string][]byte{
		"unknown kind":       {7, 0, 1, 'x'},
		"short resize":       {TermResize, 0, 2, 0, 24},
		"zero rows":          {TermResize, 0, 4, 0, 0, 0, 80},
		"rows over the max":  {TermResize, 0, 4, 0x01, 0xf5, 0, 80}, // 501
		"truncated payload":  {TermData, 0, 5, 'a'},
		"truncated header":   {TermData, 0},
		"cols over the max":  {TermResize, 0, 4, 0, 24, 0x03, 0xe9}, // 1001
		"long resize":        {TermResize, 0, 5, 0, 24, 0, 80, 0},
		"resize of all ones": {TermResize, 0, 4, 0xff, 0xff, 0xff, 0xff},
	} {
		if f, err := ReadTermFrame(bytes.NewReader(raw)); err == nil || err == io.EOF {
			t.Errorf("%s: accepted %+v (%v)", name, f, err)
		}
	}
}

// termRig serves hostd's terminal handler with the incus seams faked: the "workspace" is a
// local sh running script. It returns the server, the argv hostd built, and a channel closed
// when the handler returns.
func termRig(t *testing.T, script string) (*httptest.Server, chan []string, chan struct{}) {
	t.Helper()
	useFake(t)
	s := connectServer()
	argv := make(chan []string, 1)
	old := streamCmd
	streamCmd = func(ctx context.Context, _ place, args []string) *exec.Cmd {
		argv <- args
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
	oldHang := termHangup
	termHangup = time.Second
	t.Cleanup(func() { streamCmd, termHangup = old, oldHang })
	finished := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tenants/{t}/workspaces/{w}/terminal", func(w http.ResponseWriter, r *http.Request) {
		s.terminal(w, r)
		close(finished)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, argv, finished
}

func dialTerm(t *testing.T, srv *httptest.Server, proto, query string) (net.Conn, *bufio.Reader, *http.Response) {
	t.Helper()
	c, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	fmt.Fprintf(c, "GET /tenants/%s/workspaces/%s/terminal?%s HTTP/1.1\r\nHost: hostd\r\nConnection: Upgrade\r\nUpgrade: %s\r\n\r\n", tid, cwid, query, proto)
	br := bufio.NewReader(c)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, br, res
}

// readUntil reads output until it contains want or the deadline passes.
func readUntil(t *testing.T, c net.Conn, br *bufio.Reader, want string) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var out []byte
	buf := make([]byte, 4096)
	for !bytes.Contains(out, []byte(want)) {
		n, err := br.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			t.Fatalf("waiting for %q: %v; got %q", want, err, out)
		}
	}
	return string(out)
}

func TestTerminalRefusesUnknownUpgrade(t *testing.T) {
	srv, _, _ := termRig(t, "true")
	_, _, res := dialTerm(t, srv, "websocket", "")
	if res.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("got %d, want 426", res.StatusCode)
	}
}

// The framed stream runs the client in a host PTY: the open size, a resize frame (with the
// SIGWINCH the incus client relies on), keystrokes, and the hangup when the console goes.
func TestTerminalFramedResizeReachesPTY(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host PTY is Linux only")
	}
	srv, argv, finished := termRig(t, `trap 'echo winch; stty size' WINCH; stty size; while :; do sleep 0.05; done`)
	c, br, res := dialTerm(t, srv, TerminalFramedProto, "rows=30&cols=100&mux=plain&name=my-space")
	if res.StatusCode != http.StatusSwitchingProtocols || res.Header.Get(TerminalResizeHeader) != "pty" {
		t.Fatalf("101 with resize=pty expected: %d %q", res.StatusCode, res.Header.Get(TerminalResizeHeader))
	}
	a := <-argv
	if tail := a[len(a)-5:]; strings.Join(tail, " ") != "term 30 100 plain my-space" {
		t.Fatalf("argv tail %q", tail)
	}
	readUntil(t, c, br, "30 100")
	if err := WriteTermResize(c, 40, 120); err != nil {
		t.Fatal(err)
	}
	readUntil(t, c, br, "40 120")
	c.Close()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the shell outlived the console connection")
	}
}

func TestTerminalFramedKeystrokesAndExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host PTY is Linux only")
	}
	srv, argv, finished := termRig(t, `read l; echo "got:$l"`)
	c, br, _ := dialTerm(t, srv, TerminalFramedProto, "name=Bad;Name")
	if a := <-argv; a[len(a)-1] != "" || a[len(a)-4] != "24" || a[len(a)-3] != "80" {
		t.Fatalf("invalid name or missing size must fall back: %q", a[len(a)-5:])
	}
	if err := WriteTermData(c, []byte("hello\r")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, c, br, "got:hello")
	select { // the shell exited: hostd ends the stream without waiting for the console
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not end when the shell exited")
	}
}

func TestTerminalFramedRejectsBadFrame(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host PTY is Linux only")
	}
	srv, _, finished := termRig(t, `while :; do sleep 0.05; done`)
	c, _, _ := dialTerm(t, srv, TerminalFramedProto, "")
	if _, err := c.Write([]byte{9, 0, 0}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("an unknown frame kind must end the session")
	}
}

func TestTerminalLegacyRawStream(t *testing.T) {
	srv, _, finished := termRig(t, `read l; echo "got:$l"`)
	c, br, res := dialTerm(t, srv, TerminalProto, "rows=30&cols=100")
	if res.StatusCode != http.StatusSwitchingProtocols || res.Header.Get(TerminalResizeHeader) != "" {
		t.Fatalf("legacy 101: %d %q", res.StatusCode, res.Header.Get(TerminalResizeHeader))
	}
	fmt.Fprint(c, "raw\n")
	readUntil(t, c, br, "got:raw")
	c.Close()
	<-finished
}
