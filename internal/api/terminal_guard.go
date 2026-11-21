package api

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// termLimits are a terminal session's limits (auditor T-2): no input for idle closes the
// session; no session outlives maxLife; authorization is re-checked every recheck, so a creator
// removed from the tenant (or a workspace that stops) loses the shell within a minute.
type termLimits struct{ idle, maxLife, recheck time.Duration }

var terminalDefaults = termLimits{idle: 15 * time.Minute, maxLife: 8 * time.Hour, recheck: time.Minute}

// terminalOverride shortens the limits in tests; a session reads its limits once, at open.
var terminalOverride atomic.Pointer[termLimits]

func terminalLimits() termLimits {
	if l := terminalOverride.Load(); l != nil {
		return *l
	}
	return terminalDefaults
}

// Why a terminal session ended. The reason goes in the terminal.close audit row and, with its
// code from termCloseCodes, in the close frame the browser gets. The panel reconnects by itself
// only after "unverified" (or a drop with no close frame); every other reason needs the person.
const (
	termClosed      = "closed"      // the browser closed it
	termIdle        = "idle"        // no input for the idle limit
	termMaximum     = "maximum"     // open for the maximum lifetime
	termRevoked     = "revoked"     // the caller may no longer open this terminal
	termStopped     = "stopped"     // the workspace is no longer running
	termUnverified  = "unverified"  // the re-check could not observe the workspace
	termEnded       = "ended"       // the shell exited or hostd closed the stream
	termUnavailable = "unavailable" // hostd did not open the terminal (audit only)
)

var termCloseCodes = map[string]int{
	termClosed: 1000, termIdle: 4001, termMaximum: 4002, termRevoked: 4003,
	termStopped: 4004, termEnded: 4005, termUnverified: 4006,
}

// guardTerminal calls end with the reason when lim.maxLife passes or check (run every
// lim.recheck) returns one; call the returned func when the session ends.
func guardTerminal(lim termLimits, check func() string, end func(string)) func() {
	done := make(chan struct{})
	life := time.NewTimer(lim.maxLife)
	tick := time.NewTicker(lim.recheck)
	go func() {
		defer life.Stop()
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-life.C:
				end(termMaximum)
				return
			case <-tick.C:
				if why := check(); why != "" {
					end(why)
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// terminalStillAllowed re-runs the open-time checks (creator or manager, member, workspace
// running) and returns why the session must end, or "" while it may continue.
func (a *API) terminalStillAllowed(r *http.Request) string {
	x, ok := a.connectFor(discardWriter{}, r, false)
	switch {
	case !ok:
		return termRevoked
	case x.ws.State == "running":
		return ""
	case x.ws.State == "unknown": // hostd did not answer; fail closed, but let the panel retry
		return termUnverified
	}
	return termStopped
}

// discardWriter swallows the problem responses connectFor writes when a re-check fails.
type discardWriter struct{}

func (discardWriter) Header() http.Header         { return http.Header{} }
func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriter) WriteHeader(int)             {}
