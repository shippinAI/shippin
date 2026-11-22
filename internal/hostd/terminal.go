package hostd

import (
	"context"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TerminalProto is the Upgrade token of the console's standalone Terminal view: raw bytes both
// ways, size fixed at open. TerminalFramedProto (terminal_frame.go) adds resize.
const TerminalProto = "shippin-pty"

// termHangup bounds how long the incus client may take to exit after its terminal hangs up.
var termHangup = 5 * time.Second

// termSize reads rows/cols (bounded integers) from the query, defaulting to 24x80.
func termSize(r *http.Request) (int, int) {
	n := func(k string, def, max int) int {
		v, err := strconv.Atoi(r.URL.Query().Get(k))
		if err != nil || v < 1 || v > max {
			v = def
		}
		return v
	}
	return n("rows", 24, TermMaxRows), n("cols", 80, TermMaxCols)
}

// termName is the workspace name for the Zellij tab: a valid label or nothing.
func termName(r *http.Request) string {
	if n := r.URL.Query().Get("name"); ValidLabel(n) == nil {
		return n
	}
	return ""
}

// termScript starts the image's shippin-term (Zellij session "shippin" by default) and falls
// back to a plain login shell on images built before it existed. Older shippin-term ignores $4.
const termScript = `stty rows "$1" cols "$2" 2>/dev/null; cd; if command -v shippin-term >/dev/null 2>&1; then exec shippin-term "$3" "$4"; fi; exec bash -l`

// termMux is "plain" or "zellij" (the default); anything else is zellij.
func termMux(r *http.Request) string {
	if r.URL.Query().Get("mux") == "plain" {
		return "plain"
	}
	return "zellij"
}

// terminal (GET + Upgrade: shippin-pty or shippin-pty.2) joins the stream to a login shell run
// as the image user in a pseudo-terminal inside the workspace, with the CLI homes set. Like
// relay, only the console reaches hostd, and it authorizes (workspace creator or a tenant
// manager) and audits first. On the framed stream the incus client runs in a host PTY, so a
// resize frame reaches the workspace's PTY; without one (not Linux) resize frames are dropped.
func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	proto := r.Header.Get("Upgrade")
	framed := strings.EqualFold(proto, TerminalFramedProto)
	if !framed && !strings.EqualFold(proto, TerminalProto) {
		fail(w, http.StatusUpgradeRequired, "terminal needs Upgrade: "+TerminalFramedProto+" or "+TerminalProto)
		return
	}
	if !s.cfg.AI.enabled() || !s.cfg.Connect.enabled() {
		fail(w, http.StatusConflict, "terminal is not configured")
		return
	}
	ws := r.PathValue("w")
	at, err := s.placeOf(r.Context(), r.PathValue("t"), ws)
	if err != nil || !at.has(r.Context(), "exec", Instance(ws), "--", "true") {
		fail(w, http.StatusConflict, "workspace is not running")
		return
	}
	rows, cols := termSize(r)
	var pt *hostPTY
	if framed {
		pt, _ = openPTY(rows, cols) // nil: pipes, resize frames dropped
	}
	drop := func() {
		if pt != nil {
			pt.close()
		}
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		drop()
		fail(w, http.StatusInternalServerError, "terminal unsupported")
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		drop()
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Time{})
	head := "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + proto + "\r\n"
	if framed {
		resize := "none"
		if pt != nil {
			resize = "pty"
		}
		head += TerminalResizeHeader + ": " + resize + "\r\n"
	}
	_, _ = rw.WriteString(head + "\r\n")
	if rw.Flush() != nil {
		drop()
		return
	}
	argv := s.userExec(Instance(ws), true, "sh", "-c", termScript, "term", strconv.Itoa(rows), strconv.Itoa(cols), termMux(r), termName(r))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := streamCmd(ctx, at, argv)
	switch {
	case pt != nil:
		runTermPTY(cmd, pt, rw.Reader, conn)
	case framed:
		runTermPipes(cmd, rw.Reader, conn)
	default:
		cmd.Stdin, cmd.Stdout, cmd.Stderr = rw.Reader, conn, conn
		_ = cmd.Run() // ends when either side closes
	}
}

// runTermPTY runs cmd on the host PTY until the console goes or the shell exits, then hangs up
// the terminal (SIGHUP to the incus client) and kills the client if it lingers.
func runTermPTY(cmd *exec.Cmd, pt *hostPTY, in io.Reader, out net.Conn) {
	pt.attach(cmd)
	err := cmd.Start()
	pt.slave.Close()
	if err != nil {
		pt.master.Close()
		return
	}
	ended := make(chan struct{}, 2)
	go func() { // console -> PTY
		for {
			f, err := ReadTermFrame(in)
			if err != nil {
				break
			}
			if f.Kind == TermResize {
				_ = pt.resize(f.Rows, f.Cols)
			} else if _, err := pt.master.Write(f.Data); err != nil {
				break
			}
		}
		ended <- struct{}{}
	}()
	go func() { // PTY -> console
		_, _ = io.Copy(out, pt.master)
		ended <- struct{}{}
	}()
	<-ended
	pt.master.Close()
	out.Close()
	waitOrKill(cmd)
}

// runTermPipes is the framed stream without a PTY: data frames feed stdin, resize is dropped.
func runTermPipes(cmd *exec.Cmd, in io.Reader, out net.Conn) {
	pr, pw := io.Pipe()
	go func() {
		for {
			f, err := ReadTermFrame(in)
			if err != nil {
				break
			}
			if f.Kind == TermData {
				if _, err := pw.Write(f.Data); err != nil {
					break
				}
			}
		}
		pw.Close()
	}()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pr, out, out
	cmd.WaitDelay = time.Second
	_ = cmd.Run()
	pr.Close()
	out.Close()
}

func waitOrKill(cmd *exec.Cmd) {
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(termHangup):
		_ = cmd.Process.Kill()
		<-done
	}
}
