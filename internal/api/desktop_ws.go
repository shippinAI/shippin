package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
)

// desktopSocket is the browser's pipe to xrdp on the tenant VM loopback. Closing the tab closes
// this pipe. sesman keeps the desktop session (KillDisconnected=false), which is what Claude
// Dispatch needs. The Linux password is typed at the VM greeter; Shippin does not store it.
func (a *API) desktopSocket(w http.ResponseWriter, r *http.Request) {
	c, ok := a.appsCaller(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	detail := "tvm-" + t.ID
	if !t.member() {
		if !a.auditUse(r, c, t, "", "apps.desktop_open", detail) {
			refuseUnaudited(w)
			return
		}
	} else if err := a.db.AddAudit(r.Context(), db.AuditEntry{
		TenantID: t.ID, TenantName: t.Name, Actor: c.id.Sub, ActorName: c.id.Name,
		Action: "apps.desktop_open", Detail: detail, IP: clientIP(r),
	}); err != nil {
		refuseUnaudited(w)
		return
	}
	up, err := a.dialHostdDesktop(t.ID)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "The desktop didn't open.")
		return
	}
	defer up.Close()
	conn, rw, err := wsAccept(w, r, a.consoleOrigin())
	if err != nil {
		if conn == nil {
			auth.Problem(w, http.StatusBadRequest, "Open the desktop from the console.")
		}
		return
	}
	defer conn.Close()
	var mu sync.Mutex
	go func() {
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
	for {
		p, err := wsRead(rw.Reader, lockedWriter{conn, &mu})
		if err != nil || len(p) > 0 && writeAll(up, p) != nil {
			return
		}
	}
}

func (a *API) dialHostdDesktop(tenant string) (net.Conn, error) {
	u, err := url.Parse(a.cfg.Incus.HostdURL)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(conn, "GET /tenants/%s/desktop HTTP/1.1\r\nHost: hostd\r\nConnection: Upgrade\r\nUpgrade: %s\r\n\r\n",
		tenant, hostd.DesktopProto)
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols || br.Buffered() > 0 {
		conn.Close()
		return nil, fmt.Errorf("hostd desktop refused")
	}
	return conn, nil
}
