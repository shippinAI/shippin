package api

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

const relayProto = "shippin-relay"

// bridgeRelay upgrades the request (Upgrade: shippin-relay) into a byte stream to the workspace's
// sshd, relayed by hostd (the console has no route to workspaces). The session and RBAC are
// re-checked every bridge.recheck_seconds; revocation or lost access closes the stream. Opening
// and closing are audited. The stream still needs a CA certificate at sshd.
func (a *API) bridgeRelay(w http.ResponseWriter, r *http.Request) {
	c, ses, ok := a.bridgeCaller(w, r)
	if !ok {
		return
	}
	ws, t, ok := a.bridgeWorkspace(w, r, c, r.PathValue("id"))
	if !ok {
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), relayProto) {
		auth.Problem(w, http.StatusUpgradeRequired, "Use the Shippin bridge (Upgrade: "+relayProto+").")
		return
	}
	entry := func(act, detail string) db.AuditEntry {
		return db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: ws.ID, WorkspaceName: ws.Name, Actor: c.id.Sub,
			ActorName: c.id.Name, Action: act, Detail: detail + " session " + ses.ID, IP: clientIP(r)}
	}
	if !a.auditFirst(w, r, entry("bridge.relay_open", "ssh")) {
		return
	}
	up, err := a.dialHostdRelay(t.ID, ws.ID)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "The workspace isn't reachable right now.")
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		auth.Problem(w, http.StatusInternalServerError, "Relay unsupported.")
		return
	}
	down, rw, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	_ = down.SetDeadline(time.Time{}) // the server's read/write timeouts must not cut a live shell
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + relayProto + "\r\n\r\n")
	_ = rw.Flush()
	b := a.n2.br
	b.track(ses.ID, down)
	defer b.untrack(ses.ID, down)
	start, reason := time.Now(), atomic.Value{}
	reason.Store("closed")
	done := make(chan struct{}, 2)
	var in, out atomic.Int64
	go func() { n, _ := io.Copy(up, rw); in.Add(n); done <- struct{}{} }()
	go func() { n, _ := io.Copy(down, up); out.Add(n); done <- struct{}{} }()
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	tick := time.NewTicker(time.Duration(b.cfg.RecheckSeconds) * time.Second)
	defer tick.Stop()
loop:
	for {
		select {
		case <-done:
			break loop
		case <-tick.C:
			if !a.bridgeStillAllowed(r, tok, ws.ID) {
				reason.Store("revoked or access lost")
				break loop
			}
		}
	}
	down.Close()
	up.Close()
	detail := fmt.Sprintf("ssh %s after %s, %d B in, %d B out", reason.Load(), time.Since(start).Round(time.Second), in.Load(), out.Load())
	if err := a.store().AddAudit(r.Context(), entry("bridge.relay_close", detail)); err != nil {
		log.Printf("bridge: relay close audit failed for session %s", ses.ID)
	}
}

// bridgeStillAllowed re-runs the session and workspace checks without writing a response.
func (a *API) bridgeStillAllowed(r *http.Request, tok, wsID string) bool {
	q := r.Clone(r.Context())
	q.Header = http.Header{"Authorization": {"Bearer " + tok}}
	sink := &nopWriter{h: http.Header{}}
	c, _, ok := a.bridgeCaller(sink, q)
	if !ok {
		return false
	}
	_, _, ok = a.bridgeWorkspace(sink, q, c, wsID)
	return ok
}

type nopWriter struct{ h http.Header }

func (n *nopWriter) Header() http.Header         { return n.h }
func (n *nopWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *nopWriter) WriteHeader(int)             {}

// dialHostdRelay opens hostd's relay for the workspace and returns the upgraded connection.
func (a *API) dialHostdRelay(tenant, ws string) (net.Conn, error) {
	u, err := url.Parse(a.cfg.Incus.HostdURL)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(conn, "GET /tenants/%s/workspaces/%s/relay HTTP/1.1\r\nHost: hostd\r\nConnection: Upgrade\r\nUpgrade: %s\r\n\r\n", tenant, ws, relayProto)
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("hostd relay refused")
	}
	if br.Buffered() > 0 {
		conn.Close()
		return nil, fmt.Errorf("hostd relay sent early data")
	}
	return conn, nil
}
