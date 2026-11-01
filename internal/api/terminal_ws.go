package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostd"
)

// terminalSocket is the standalone Terminal view: a same-origin WebSocket bridged to hostd's
// PTY stream for the workspace (creator only, audited before the shell opens).
func (a *API) terminalSocket(w http.ResponseWriter, r *http.Request) {
	// origin first (F-1): a cross-site page must not open (and audit) a PTY before being refused
	if r.Header.Get("Origin") != a.consoleOrigin() {
		auth.Problem(w, http.StatusForbidden, "Open the terminal from the console.")
		return
	}
	x, ok := a.connectFor(w, r, false)
	if !ok {
		return
	}
	if x.ws.State != "running" {
		auth.Problem(w, http.StatusConflict, "Start the workspace to open its terminal.")
		return
	}
	if !a.connectAudit(r, x, "terminal.open") {
		refuseUnaudited(w)
		return
	}
	up, err := a.dialHostdTerminal(x.t.ID, x.ws.ID, r.URL.Query().Get("rows"), r.URL.Query().Get("cols"), r.URL.Query().Get("mux") == "plain")
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "The terminal didn't open.")
		return
	}
	defer up.Close()
	conn, rw, err := wsAccept(w, r, a.consoleOrigin())
	if err != nil {
		if conn == nil {
			auth.Problem(w, http.StatusBadRequest, "Open the terminal from the console.")
		}
		return
	}
	defer conn.Close()
	stop := a.guardTerminal(r, conn, up)
	defer stop()
	var mu sync.Mutex
	go func() { // workspace -> browser
		buf := make([]byte, 32<<10)
		for {
			n, err := up.Read(buf)
			if n > 0 {
				mu.Lock()
				werr := wsWrite(conn, 0x2, buf[:n])
				mu.Unlock()
				if werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		conn.Close()
	}()
	for { // browser -> workspace
		_ = conn.SetReadDeadline(time.Now().Add(terminalIdle))
		p, err := wsRead(rw.Reader, lockedWriter{conn, &mu})
		if err != nil || len(p) > 0 && writeAll(up, p) != nil {
			return
		}
	}
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

// dialHostdTerminal opens hostd's PTY stream; rows/cols are re-parsed as integers here and the
// multiplexer is a fixed token (plain shell or the Zellij default), never caller text.
func (a *API) dialHostdTerminal(tenant, ws, rows, cols string, plain bool) (net.Conn, error) {
	u, err := url.Parse(a.cfg.Incus.HostdURL)
	if err != nil {
		return nil, err
	}
	rn, _ := strconv.Atoi(rows)
	cn, _ := strconv.Atoi(cols)
	mux := "zellij"
	if plain {
		mux = "plain"
	}
	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(conn, "GET /tenants/%s/workspaces/%s/terminal?rows=%d&cols=%d&mux=%s HTTP/1.1\r\nHost: hostd\r\nConnection: Upgrade\r\nUpgrade: %s\r\n\r\n",
		tenant, ws, rn, cn, mux, hostd.TerminalProto)
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols || br.Buffered() > 0 {
		conn.Close()
		return nil, fmt.Errorf("hostd terminal refused")
	}
	return conn, nil
}
