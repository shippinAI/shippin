package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
)

// terminalSocket is the standalone Terminal view: a same-origin WebSocket bridged to hostd's
// PTY stream for the workspace (creator or tenant manager only, audited before the shell
// opens, and audited again when it closes with how long it ran and why it ended).
//
// Browser -> console: binary frames are keystrokes; a text frame is a control message,
// {"type":"resize","rows":R,"cols":C}. Console -> browser: binary frames are output; the first
// frame is text, {"type":"ready","resize":bool}, saying whether a resize reaches the shell. The
// session ends with a close frame whose code says why (termCloseCodes). A refusal after the
// origin check is a close frame too, 4000 + the HTTP status with the message as the reason,
// because a browser cannot read the status of a refused WebSocket handshake.
func (a *API) terminalSocket(w http.ResponseWriter, r *http.Request) {
	// origin first (F-1): a cross-site page must not open (and audit) a PTY before being refused
	if r.Header.Get("Origin") != a.consoleOrigin() {
		auth.Problem(w, http.StatusForbidden, "Open the terminal from the console.")
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		auth.Problem(w, http.StatusBadRequest, "Open the terminal from the console.")
		return
	}
	var refused problemCapture
	x, ok := a.connectFor(&refused, r, false)
	if !ok {
		a.refuseTerminal(w, r, refused.status, refused.message())
		return
	}
	if x.ws.State != "running" {
		a.refuseTerminal(w, r, http.StatusConflict, "Start the workspace to open its terminal.")
		return
	}
	if !a.connectAudit(r, x, "terminal.open") {
		a.refuseTerminal(w, r, http.StatusServiceUnavailable, "Access couldn't be recorded in the audit log, so it was refused.")
		return
	}
	opened := time.Now()
	q := r.URL.Query()
	up, err := a.dialHostdTerminal(x.t.ID, x.ws.ID, q.Get("rows"), q.Get("cols"), q.Get("mux") == "plain", x.ws.Name)
	if err != nil {
		a.auditTerminalClose(r, x, termUnavailable, 0)
		a.refuseTerminal(w, r, http.StatusBadGateway, "The terminal didn't open.")
		return
	}
	defer up.Close()
	conn, rw, err := wsAccept(w, r, a.consoleOrigin())
	if err != nil {
		if conn == nil {
			auth.Problem(w, http.StatusBadRequest, "Open the terminal from the console.")
		}
		a.auditTerminalClose(r, x, termClosed, time.Since(opened))
		return
	}
	defer conn.Close()
	why := a.pipeTerminal(r, conn, rw.Reader, up)
	a.auditTerminalClose(r, x, why, time.Since(opened))
}

// pipeTerminal joins the browser and hostd until one side goes, the guard ends the session or
// the browser sends no input for the idle limit, and returns why it ended.
func (a *API) pipeTerminal(r *http.Request, conn net.Conn, br *bufio.Reader, up *hostdTerm) string {
	var mu sync.Mutex
	e := &termEnd{conn: conn, up: up, mu: &mu}
	lim := terminalLimits()
	stop := guardTerminal(lim, func() string { return a.terminalStillAllowed(r) }, e.end)
	defer stop()
	mu.Lock()
	err := wsWrite(conn, 0x1, termReady(up.resize))
	mu.Unlock()
	if err != nil {
		e.end(termClosed)
		return e.reason
	}
	go func() { // workspace -> browser
		buf := make([]byte, 32<<10)
		for {
			n, err := up.Read(buf)
			if n > 0 {
				mu.Lock()
				werr := wsWrite(conn, 0x2, buf[:n])
				mu.Unlock()
				if werr != nil {
					e.end(termClosed)
					return
				}
			}
			if err != nil {
				e.end(termEnded)
				return
			}
		}
	}()
	idleAt := time.Now().Add(lim.idle) // only keystrokes count as activity, not resizes
	for {                                  // browser -> workspace
		_ = conn.SetReadDeadline(idleAt)
		op, p, err := wsReadOp(br, lockedWriter{conn, &mu})
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				e.end(termIdle)
			} else {
				e.end(termClosed)
			}
			return e.reason
		}
		if op == 0x1 {
			if rows, cols, ok := parseTermControl(p); ok && up.setSize(rows, cols) != nil {
				e.end(termEnded)
				return e.reason
			}
			continue
		}
		idleAt = time.Now().Add(lim.idle)
		if len(p) > 0 && up.input(p) != nil {
			e.end(termEnded)
			return e.reason
		}
	}
}

// termEnd ends a session once: it records why, tells the browser in a close frame, and closes
// both ends. reason is read only by a goroutine that has itself called end.
type termEnd struct {
	once   sync.Once
	reason string
	conn   net.Conn
	up     io.Closer
	mu     *sync.Mutex
}

func (e *termEnd) end(reason string) {
	e.once.Do(func() {
		e.reason = reason
		// bound a write stuck on a browser that stopped reading, so the lock frees and the
		// session still closes (revocation must not wait on the client)
		_ = e.conn.SetWriteDeadline(time.Now().Add(time.Second))
		e.mu.Lock()
		_ = wsClose(e.conn, termCloseCodes[reason], reason)
		e.mu.Unlock()
		e.conn.Close()
		e.up.Close()
	})
}

// termReady is the first frame: whether a resize reaches the workspace's PTY.
func termReady(resize bool) []byte {
	b, _ := json.Marshal(map[string]any{"type": "ready", "resize": resize})
	return b
}

// parseTermControl reads a browser control message; only an in-bounds resize is accepted.
func parseTermControl(p []byte) (rows, cols int, ok bool) {
	var m struct {
		Type       string
		Rows, Cols int
	}
	if json.Unmarshal(p, &m) != nil || m.Type != "resize" || !hostd.ValidTermSize(m.Rows, m.Cols) {
		return 0, 0, false
	}
	return m.Rows, m.Cols, true
}

// refuseTerminal refuses after the origin check: on a WebSocket handshake it accepts and sends
// a close frame (4000 + status, the message as reason) so the panel can say why and stop
// retrying; otherwise it is the usual problem response.
func (a *API) refuseTerminal(w http.ResponseWriter, r *http.Request, status int, msg string) {
	conn, _, err := wsAccept(w, r, a.consoleOrigin())
	if err != nil {
		if conn == nil {
			auth.Problem(w, status, msg)
		}
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	_ = wsClose(conn, 4000+status, msg)
}

// auditTerminalClose records terminal.close: why the session ended and how long it was open.
// The request's context may already be done, so the row is written on its own deadline.
func (a *API) auditTerminalClose(r *http.Request, x *connectCtx, why string, open time.Duration) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	err := a.db.AddAudit(ctx, db.AuditEntry{TenantID: x.t.ID, WorkspaceID: x.ws.ID, Actor: x.c.id.Sub,
		ActorName: x.c.id.Name, Action: "terminal.close", IP: clientIP(r),
		Detail: fmt.Sprintf("reason=%s duration=%s", why, open.Round(time.Second))})
	if err != nil {
		log.Printf("terminal.close audit for workspace %s: %v", x.ws.ID, err)
	}
}

// problemCapture keeps the status and message of the problem connectFor writes on a refusal.
type problemCapture struct {
	status int
	body   []byte
}

func (p *problemCapture) Header() http.Header         { return http.Header{} }
func (p *problemCapture) Write(b []byte) (int, error) { p.body = append(p.body, b...); return len(b), nil }
func (p *problemCapture) WriteHeader(s int)           { p.status = s }

func (p *problemCapture) message() string {
	var m struct{ Message string }
	if json.Unmarshal(p.body, &m) != nil || m.Message == "" {
		return http.StatusText(p.status)
	}
	return m.Message
}

type lockedWriter struct {
	c  net.Conn
	mu *sync.Mutex
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.c.Write(p)
}

func writeAll(c net.Conn, p []byte) error { _, err := c.Write(p); return err }

// hostdTerm is hostd's PTY stream. framed: hostd speaks shippin-pty.2, so input is framed and
// a resize can be sent; resize: hostd runs the client in a host PTY, so a resize reaches the
// shell. Against a hostd from before the framed stream both are false and resize is dropped.
type hostdTerm struct {
	net.Conn
	br             *bufio.Reader // holds any output that arrived with the 101
	framed, resize bool
}

func (h *hostdTerm) Read(p []byte) (int, error) { return h.br.Read(p) }

func (h *hostdTerm) input(p []byte) error {
	if h.framed {
		return hostd.WriteTermData(h.Conn, p)
	}
	return writeAll(h.Conn, p)
}

func (h *hostdTerm) setSize(rows, cols int) error {
	if !h.framed {
		return nil
	}
	return hostd.WriteTermResize(h.Conn, rows, cols)
}

// dialHostdTerminal opens hostd's PTY stream, framed when hostd supports it. rows/cols are
// re-parsed as integers here, the multiplexer is a fixed token (plain shell or the Zellij
// default) and the name is sent only when it is a valid workspace label, never other caller text.
func (a *API) dialHostdTerminal(tenant, ws, rows, cols string, plain bool, name string) (*hostdTerm, error) {
	rn, _ := strconv.Atoi(rows)
	cn, _ := strconv.Atoi(cols)
	mux := "zellij"
	if plain {
		mux = "plain"
	}
	q := fmt.Sprintf("rows=%d&cols=%d&mux=%s", rn, cn, mux)
	if hostd.ValidLabel(name) == nil {
		q += "&name=" + url.QueryEscape(name)
	}
	path := fmt.Sprintf("/tenants/%s/workspaces/%s/terminal?%s", tenant, ws, q)
	t, status, err := a.dialHostdTerm(path, hostd.TerminalFramedProto)
	if status == http.StatusUpgradeRequired { // a hostd from before the framed stream
		t, _, err = a.dialHostdTerm(path, hostd.TerminalProto)
	}
	return t, err
}

func (a *API) dialHostdTerm(path, proto string) (*hostdTerm, int, error) {
	u, err := url.Parse(a.cfg.Incus.HostdURL)
	if err != nil {
		return nil, 0, err
	}
	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return nil, 0, err
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: hostd\r\nConnection: Upgrade\r\nUpgrade: %s\r\n\r\n", path, proto)
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		return nil, status, fmt.Errorf("hostd terminal refused")
	}
	framed := proto == hostd.TerminalFramedProto
	return &hostdTerm{Conn: conn, br: br, framed: framed, resize: framed && res.Header.Get(hostd.TerminalResizeHeader) == "pty"}, 0, nil
}
