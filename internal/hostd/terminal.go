package hostd

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TerminalProto is the Upgrade token of the console's standalone Terminal view.
const TerminalProto = "shippin-pty"

// termSize reads rows/cols (bounded integers) from the query, defaulting to 24x80.
func termSize(r *http.Request) (string, string) {
	n := func(k string, def, max int) string {
		v, err := strconv.Atoi(r.URL.Query().Get(k))
		if err != nil || v < 1 || v > max {
			v = def
		}
		return strconv.Itoa(v)
	}
	return n("rows", 24, 500), n("cols", 80, 1000)
}

// termScript starts the image's shippin-term (Zellij session "shippin" by default) and falls
// back to a plain login shell on images built before it existed.
const termScript = `stty rows "$1" cols "$2" 2>/dev/null; cd; if command -v shippin-term >/dev/null 2>&1; then exec shippin-term "$3"; fi; exec bash -l`

// termMux is "plain" or "zellij" (the default); anything else is zellij.
func termMux(r *http.Request) string {
	if r.URL.Query().Get("mux") == "plain" {
		return "plain"
	}
	return "zellij"
}

// terminal (GET + Upgrade: shippin-pty) joins the stream to a login shell run as the image user
// in a pseudo-terminal inside the workspace, with the CLI homes set. Like relay, only the
// console reaches hostd, and it authorizes (workspace creator only) and audits first.
func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), TerminalProto) {
		fail(w, http.StatusUpgradeRequired, "terminal needs Upgrade: "+TerminalProto)
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
	hj, ok := w.(http.Hijacker)
	if !ok {
		fail(w, http.StatusInternalServerError, "terminal unsupported")
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Time{})
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + TerminalProto + "\r\n\r\n")
	if rw.Flush() != nil {
		return
	}
	rows, cols := termSize(r)
	argv := s.userExec(Instance(ws), true, "sh", "-c", termScript, "term", rows, cols, termMux(r))
	cmd := streamCmd(context.Background(), at, argv)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = rw.Reader, conn, conn
	_ = cmd.Run() // ends when either side closes
}
