package api

import (
	"net"
	"net/http"
	"time"
)

// Terminal session limits (auditor T-2): no input for terminalIdle closes the session; no
// session outlives terminalMaxLife; authorization is re-checked every terminalRecheck, so a
// creator removed from the tenant (or a workspace that stops) loses the shell within a minute.
var (
	terminalIdle    = 15 * time.Minute
	terminalMaxLife = 8 * time.Hour
	terminalRecheck = time.Minute
)

// guardTerminal closes both ends when the lifetime ends or the re-check fails; call the
// returned func when the session ends.
func (a *API) guardTerminal(r *http.Request, conn, up net.Conn) func() {
	done := make(chan struct{})
	go func() {
		life := time.NewTimer(terminalMaxLife)
		tick := time.NewTicker(terminalRecheck)
		defer life.Stop()
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-life.C:
			case <-tick.C:
				if a.terminalStillAllowed(r) {
					continue
				}
			}
			conn.Close()
			up.Close()
			return
		}
	}()
	return func() { close(done) }
}

// terminalStillAllowed re-runs the open-time checks (creator, member, workspace running).
func (a *API) terminalStillAllowed(r *http.Request) bool {
	x, ok := a.connectFor(discardWriter{}, r, false)
	return ok && x.ws.State == "running"
}

// discardWriter swallows the problem responses connectFor writes when a re-check fails.
type discardWriter struct{}

func (discardWriter) Header() http.Header         { return http.Header{} }
func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriter) WriteHeader(int)             {}
