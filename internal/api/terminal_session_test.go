package api

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"shippin.cloud/console/internal/infrastructure/hostd"
)

// wsClientFrame is one masked client frame, as a browser sends it.
func wsClientFrame(op byte, p []byte) []byte {
	h := []byte{0x80 | op}
	switch n := len(p); {
	case n < 126:
		h = append(h, 0x80|byte(n))
	case n <= 0xffff:
		h = append(h, 0x80|126, byte(n>>8), byte(n))
	default:
		h = append(h, 0x80|127)
		h = binary.BigEndian.AppendUint64(h, uint64(n))
	}
	mask := [4]byte{9, 8, 7, 6}
	h = append(h, mask[:]...)
	for i, b := range p {
		h = append(h, b^mask[i%4])
	}
	return h
}

// wsServerFrame reads one unmasked server frame.
func wsServerFrame(t *testing.T, r io.Reader) (byte, []byte) {
	t.Helper()
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		t.Fatalf("server frame: %v", err)
	}
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		_, _ = io.ReadFull(r, b[:])
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		_, _ = io.ReadFull(r, b[:])
		n = binary.BigEndian.Uint64(b[:])
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		t.Fatalf("server frame payload: %v", err)
	}
	return h[0] & 0x0f, p
}

// wsCloseOf splits a close payload into code and reason.
func wsCloseOf(p []byte) (int, string) {
	if len(p) < 2 {
		return 0, ""
	}
	return int(binary.BigEndian.Uint16(p[:2])), string(p[2:])
}

func TestParseTermControl(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"type":"resize","rows":40,"cols":120}`:   true,
		`{"type":"resize","rows":500,"cols":1000}`: true,
		`{"type":"resize","rows":0,"cols":120}`:    false,
		`{"type":"resize","rows":40,"cols":1001}`:  false,
		`{"type":"resize","rows":-1,"cols":80}`:    false,
		`{"type":"ready","rows":40,"cols":120}`:    false,
		`{"type":"resize"}`:                        false,
		`resize 40 120`:                            false,
		``:                                         false,
	} {
		rows, cols, ok := parseTermControl([]byte(raw))
		if ok != want || ok && (rows != 40 && rows != 500) {
			t.Errorf("%q: got %d x %d %v, want ok=%v", raw, rows, cols, ok, want)
		}
	}
}

func TestWSReadOpAndClose(t *testing.T) {
	var wire bytes.Buffer
	wire.Write(wsClientFrame(0x1, []byte(`{"type":"resize"}`)))
	wire.Write(wsClientFrame(0x2, []byte("ls\r")))
	wire.Write(wsClientFrame(0x0, []byte("more")))
	wire.Write(wsClientFrame(0x8, nil))
	br := bufio.NewReader(&wire)
	for _, want := range []struct {
		op byte
		p  string
	}{{0x1, `{"type":"resize"}`}, {0x2, "ls\r"}, {0x2, "more"}} {
		op, p, err := wsReadOp(br, io.Discard)
		if err != nil || op != want.op || string(p) != want.p {
			t.Fatalf("got %x %q %v, want %x %q", op, p, err, want.op, want.p)
		}
	}
	if _, _, err := wsReadOp(br, io.Discard); err != io.EOF {
		t.Fatalf("close frame must end the stream: %v", err)
	}
	var out bytes.Buffer
	long := strings.Repeat("é", 100) // 200 bytes: must be cut on a character boundary
	if err := wsClose(&out, 4403, long); err != nil {
		t.Fatal(err)
	}
	op, p := wsServerFrame(t, &out)
	code, reason := wsCloseOf(p)
	if op != 0x8 || code != 4403 || len(p) > 125 || !utf8.ValidString(reason) {
		t.Fatalf("close frame op %x code %d len %d valid %v", op, code, len(p), utf8.ValidString(reason))
	}
}

func TestGuardTerminal(t *testing.T) {
	wait := func(ch chan string) string {
		select {
		case why := <-ch:
			return why
		case <-time.After(3 * time.Second):
			return "timeout"
		}
	}
	// maximum lifetime, while every re-check passes
	lim := termLimits{idle: time.Hour, maxLife: 150 * time.Millisecond, recheck: 20 * time.Millisecond}
	ended, checks := make(chan string, 1), 0
	stop := guardTerminal(lim, func() string { checks++; return "" }, func(why string) { ended <- why })
	if why := wait(ended); why != termMaximum || checks == 0 {
		t.Fatalf("got %q after %d checks, want maximum after re-checks", why, checks)
	}
	stop()
	// a failed re-check ends it with the re-check's reason
	lim.maxLife = time.Hour
	for _, reason := range []string{termRevoked, termStopped, termUnverified} {
		ended = make(chan string, 1)
		n := 0
		stop = guardTerminal(lim, func() string {
			if n++; n < 3 {
				return ""
			}
			return reason
		}, func(why string) { ended <- why })
		if why := wait(ended); why != reason {
			t.Fatalf("got %q, want %q", why, reason)
		}
		stop()
	}
	// stopped: nothing more happens, and stopping twice is safe
	ended = make(chan string, 1)
	stop = guardTerminal(lim, func() string { return termRevoked }, func(why string) { ended <- why })
	stop()
	stop()
	select {
	case why := <-ended:
		t.Fatalf("a stopped guard ended the session: %q", why)
	case <-time.After(100 * time.Millisecond):
	}
}

// useTermLimits sets the limits sessions opened during the test read.
func useTermLimits(t *testing.T, l termLimits) {
	terminalOverride.Store(&l)
	t.Cleanup(func() { terminalOverride.Store(nil) })
}

// pipeRig runs pipeTerminal between net.Pipe ends: the test plays the browser (bc) and hostd (hs).
type pipeRig struct {
	bc, hs net.Conn
	why    chan string
	fromWS chan []byte // everything hostd received, in order
}

func newPipeRig(t *testing.T, framed, resize bool) *pipeRig {
	t.Helper()
	bc, sc := net.Pipe()
	hc, hs := net.Pipe()
	p := &pipeRig{bc: bc, hs: hs, why: make(chan string, 1), fromWS: make(chan []byte, 64)}
	t.Cleanup(func() { bc.Close(); hs.Close() })
	up := &hostdTerm{Conn: hc, br: bufio.NewReader(hc), framed: framed, resize: resize}
	r := httptest.NewRequest("GET", "/v1/tenants/t/workspaces/w/terminal", nil)
	go func() { p.why <- (&API{}).pipeTerminal(r, sc, bufio.NewReader(sc), up) }()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := hs.Read(buf)
			if n > 0 {
				p.fromWS <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(p.fromWS)
				return
			}
		}
	}()
	op, ready := wsServerFrame(t, bc)
	var m struct {
		Type   string
		Resize bool
	}
	if op != 0x1 || json.Unmarshal(ready, &m) != nil || m.Type != "ready" || m.Resize != resize {
		t.Fatalf("ready frame %x %s", op, ready)
	}
	return p
}

// closeOf reads frames until the close frame and returns its code and reason.
func (p *pipeRig) closeOf(t *testing.T) (int, string) {
	t.Helper()
	_ = p.bc.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		op, b := wsServerFrame(t, p.bc)
		if op == 0x8 {
			return wsCloseOf(b)
		}
	}
}

func (p *pipeRig) reason(t *testing.T) string {
	t.Helper()
	select {
	case why := <-p.why:
		return why
	case <-time.After(3 * time.Second):
		t.Fatal("session did not end")
		return ""
	}
}

// hostdGot collects what hostd received until want bytes have arrived.
func (p *pipeRig) hostdGot(t *testing.T, want int) []byte {
	t.Helper()
	var got []byte
	for len(got) < want {
		select {
		case b, ok := <-p.fromWS:
			if !ok {
				return got
			}
			got = append(got, b...)
		case <-time.After(3 * time.Second):
			t.Fatalf("hostd got %q, want %d bytes", got, want)
		}
	}
	return got
}

func TestPipeTerminalForwardsInputAndResize(t *testing.T) {
	useTermLimits(t, termLimits{idle: time.Hour, maxLife: time.Hour, recheck: time.Hour})
	p := newPipeRig(t, true, true)
	p.bc.Write(wsClientFrame(0x2, []byte("ls\r")))
	p.bc.Write(wsClientFrame(0x1, []byte(`{"type":"resize","rows":40,"cols":120}`)))
	p.bc.Write(wsClientFrame(0x1, []byte(`{"type":"resize","rows":9999,"cols":120}`))) // dropped
	p.bc.Write(wsClientFrame(0x1, []byte(`not json`)))                                 // dropped
	p.bc.Write(wsClientFrame(0x2, []byte("x")))
	var want bytes.Buffer
	_ = hostd.WriteTermData(&want, []byte("ls\r"))
	_ = hostd.WriteTermResize(&want, 40, 120)
	_ = hostd.WriteTermData(&want, []byte("x"))
	if got := p.hostdGot(t, want.Len()); !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("hostd got % x\nwant % x", got, want.Bytes())
	}
	p.hs.Write([]byte("output"))
	if op, b := wsServerFrame(t, p.bc); op != 0x2 || string(b) != "output" {
		t.Fatalf("output frame %x %q", op, b)
	}
	p.bc.Write(wsClientFrame(0x8, nil))
	if code, _ := p.closeOf(t); code != 1000 {
		t.Fatalf("close code %d", code)
	}
	if why := p.reason(t); why != termClosed {
		t.Fatalf("reason %q", why)
	}
}

func TestPipeTerminalLegacyHostdDropsResize(t *testing.T) {
	useTermLimits(t, termLimits{idle: time.Hour, maxLife: time.Hour, recheck: time.Hour})
	p := newPipeRig(t, false, false)
	p.bc.Write(wsClientFrame(0x1, []byte(`{"type":"resize","rows":40,"cols":120}`)))
	p.bc.Write(wsClientFrame(0x2, []byte("raw")))
	if got := p.hostdGot(t, 3); string(got) != "raw" {
		t.Fatalf("legacy hostd got %q", got)
	}
	p.bc.Close()
	if why := p.reason(t); why != termClosed {
		t.Fatalf("reason %q", why)
	}
}

// Idle counts keystrokes only: a pane that keeps resizing still closes.
func TestPipeTerminalIdle(t *testing.T) {
	useTermLimits(t, termLimits{idle: 300 * time.Millisecond, maxLife: time.Hour, recheck: time.Hour})
	p := newPipeRig(t, true, true)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(50 * time.Millisecond):
				if _, err := p.bc.Write(wsClientFrame(0x1, []byte(`{"type":"resize","rows":30,"cols":90}`))); err != nil {
					return
				}
			}
		}
	}()
	defer close(done)
	if code, reason := p.closeOf(t); code != 4001 || reason != termIdle {
		t.Fatalf("close %d %q", code, reason)
	}
	if why := p.reason(t); why != termIdle {
		t.Fatalf("reason %q", why)
	}
}

func TestPipeTerminalShellEnded(t *testing.T) {
	useTermLimits(t, termLimits{idle: time.Hour, maxLife: time.Hour, recheck: time.Hour})
	p := newPipeRig(t, true, true)
	p.hs.Close()
	if code, reason := p.closeOf(t); code != 4005 || reason != termEnded {
		t.Fatalf("close %d %q", code, reason)
	}
	if why := p.reason(t); why != termEnded {
		t.Fatalf("reason %q", why)
	}
}

// A browser that stops reading cannot hold the session open past its end.
func TestTermEndDoesNotWaitForAStuckBrowser(t *testing.T) {
	bc, sc := net.Pipe()
	hc, hs := net.Pipe()
	defer bc.Close()
	defer hs.Close()
	var mu sync.Mutex
	e := &termEnd{conn: sc, up: hc, mu: &mu}
	go func() { // output stuck: nobody reads bc
		mu.Lock()
		_ = wsWrite(sc, 0x2, []byte("x"))
		mu.Unlock()
	}()
	time.Sleep(20 * time.Millisecond)
	done := make(chan struct{})
	go func() { e.end(termRevoked); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("end blocked on a browser that stopped reading")
	}
	if e.reason != termRevoked {
		t.Fatalf("reason %q", e.reason)
	}
}
