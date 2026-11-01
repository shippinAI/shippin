package hostd

import (
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// relayScript joins stdin/stdout to the workspace's own sshd on loopback. It runs inside the
// workspace through incus exec, so it works for legacy and tenant-VM placement alike and needs no
// route or open port; sshd itself still demands a Shippin CA certificate.
const relayScript = `exec 3<>/dev/tcp/127.0.0.1/22 || exit 1; cat <&3 & cat >&3; kill $! 2>/dev/null`

// relay (GET + Upgrade: shippin-relay) streams bytes to the workspace's sshd. Only the console
// reaches hostd (its socket proxy); the console authorizes and audits every relay first.
func (s *Server) relay(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "shippin-relay") {
		fail(w, http.StatusUpgradeRequired, "relay needs Upgrade: shippin-relay")
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
		fail(w, http.StatusInternalServerError, "relay unsupported")
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Time{})
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: shippin-relay\r\n\r\n")
	if rw.Flush() != nil {
		return
	}
	cmd := exec.Command("incus", at.argv([]string{"exec", "--force-noninteractive", Instance(ws), "--", "bash", "-c", relayScript})...)
	cmd.Stdin, cmd.Stdout = rw.Reader, conn
	_ = cmd.Run() // ends when either side closes
}
