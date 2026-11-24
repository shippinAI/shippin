package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"shippin.cloud/console/internal/runtime/devspace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/infrastructure/hostclient"
	"shippin.cloud/console/internal/infrastructure/hostd"
)

// fakeHostd answers the inventory and the terminal upgrade the way hostd does. The terminal
// greets with "welcome" in the same write as the 101 and records every frame it receives.
type fakeHostd struct {
	mu       sync.Mutex
	status   map[string]string // instance -> status; absent = not listed
	down     atomic.Bool       // inventory fails (hostd unobservable)
	legacy   bool              // a hostd from before the framed stream
	refuse   bool              // the terminal upgrade fails
	dials    atomic.Int32
	frames   chan hostd.TermFrame
	raw      chan []byte
	sessions sync.WaitGroup
}

func (f *fakeHostd) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tenants/{t}/instances", func(w http.ResponseWriter, r *http.Request) {
		if f.down.Load() {
			http.Error(w, `{"error":"down"}`, http.StatusBadGateway)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		items := []map[string]string{}
		for name, st := range f.status {
			items = append(items, map[string]string{"name": name, "status": st})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": items, "partial": false})
	})
	mux.HandleFunc("GET /tenants/{t}/workspaces/{w}/terminal", func(w http.ResponseWriter, r *http.Request) {
		f.dials.Add(1)
		proto := r.Header.Get("Upgrade")
		if f.refuse || (f.legacy && proto != hostd.TerminalProto) {
			code := http.StatusBadGateway
			if f.legacy {
				code = http.StatusUpgradeRequired
			}
			http.Error(w, "no", code)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		f.sessions.Add(1)
		defer f.sessions.Done()
		defer conn.Close()
		head := "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + proto + "\r\n"
		if proto == hostd.TerminalFramedProto {
			head += hostd.TerminalResizeHeader + ": pty\r\n"
		}
		_, _ = rw.WriteString(head + "\r\nwelcome")
		_ = rw.Flush()
		if proto != hostd.TerminalFramedProto {
			buf := make([]byte, 1024)
			for {
				n, err := rw.Read(buf)
				if n > 0 {
					f.raw <- append([]byte(nil), buf[:n]...)
				}
				if err != nil {
					return
				}
			}
		}
		for {
			fr, err := hostd.ReadTermFrame(rw.Reader)
			if err != nil {
				return
			}
			f.frames <- fr
		}
	})
	return mux
}

type termDB struct {
	a        *API
	d        *db.DB
	tn, tn2  db.Tenant
	mine     *devspace.Devspace // tenant tn, created by "creator"
	other    *devspace.Devspace // tenant tn, created by "someone-else"
	foreign  *devspace.Devspace // tenant tn2, created by "creator"
	hostd    *fakeHostd
	console  *httptest.Server
	who      atomic.Pointer[caller]
	hostdURL string
}

func termDBRig(t *testing.T) *termDB {
	t.Helper()
	a, d, tn := notifyAPI(t)
	ctx := context.Background()
	tn2 := db.Tenant{ID: newUUID(), IdpOrgID: newUUID(), Name: "Other", Plan: "p"}
	if err := d.CreateTenant(ctx, tn2); err != nil {
		t.Fatal(err)
	}
	x := &termDB{a: a, d: d, tn: tn, tn2: tn2}
	f := &fakeHostd{status: map[string]string{}, frames: make(chan hostd.TermFrame, 64), raw: make(chan []byte, 64)}
	x.hostd = f
	mk := func(tenant db.Tenant, by string) *devspace.Devspace {
		id := newUUID()
		ws := &devspace.Devspace{ID: id, TenantID: tenant.ID, Name: "w" + id[len(id)-8:], CreatedBy: by, Host: id + ".ws.test"}
		if err := devspace.On(d.Pool).Create(ctx, ws, a.cfg.Workspaces.PortMin, a.cfg.Workspaces.PortMax); err != nil {
			t.Fatal(err)
		}
		if err := devspace.On(d.Pool).SetState(ctx, id, "running", "", ""); err != nil {
			t.Fatal(err)
		}
		f.status["w-"+id] = "Running"
		return ws
	}
	x.mine, x.other, x.foreign = mk(tn, "creator"), mk(tn, "someone-else"), mk(tn2, "creator")
	hs := httptest.NewServer(f.handler())
	t.Cleanup(hs.Close)
	t.Cleanup(f.sessions.Wait)
	a.cfg.Incus.HostdURL, a.host = hs.URL, hostclient.New(hs.URL)
	a.cfg.Connect.Enabled = true
	x.as("creator", "org_member")
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return x.who.Load(), true }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces/{workspaceId}/terminal", a.terminalSocket)
	x.console = httptest.NewServer(mux)
	t.Cleanup(x.console.Close)
	return x
}

// as makes the caller sub, a member of tn with role (or of no tenant when role is "").
func (x *termDB) as(sub, role string) {
	c := &caller{id: &auth.Identity{Sub: sub, Name: "User " + sub}, managers: []string{"org_owner", "org_admin"}}
	if role != "" {
		c.tenants = []tenantView{{Tenant: x.tn, Roles: []string{role}}}
	}
	x.who.Store(c)
}

// open does the browser's handshake for tenant/workspace and returns the socket.
func (x *termDB) open(t *testing.T, tenant, ws string) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.Dial("tcp", strings.TrimPrefix(x.console.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	fmt.Fprintf(c, "GET /v1/tenants/%s/workspaces/%s/terminal?rows=30&cols=100&mux=zellij HTTP/1.1\r\nHost: console.test\r\n"+
		"Origin: https://console.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", tenant, ws)
	br := bufio.NewReader(c)
	res, err := http.ReadResponse(br, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake: %v %v", res, err)
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	return c, br
}

// closeFrame reads until the close frame, returning its code and reason.
func closeFrame(t *testing.T, br *bufio.Reader) (int, string) {
	t.Helper()
	for {
		op, p := wsServerFrame(t, br)
		if op == 0x8 {
			return wsCloseOf(p)
		}
	}
}

// ready reads the ready frame and the greeting hostd sent with its 101.
func ready(t *testing.T, br *bufio.Reader, resize bool) {
	t.Helper()
	op, p := wsServerFrame(t, br)
	if op != 0x1 || string(p) != fmt.Sprintf(`{"resize":%v,"type":"ready"}`, resize) {
		t.Fatalf("ready frame %x %s", op, p)
	}
	if op, p := wsServerFrame(t, br); op != 0x2 || string(p) != "welcome" {
		t.Fatalf("greeting %x %q", op, p)
	}
}

// audit waits for the workspace's terminal rows (open and close) and returns their details.
func (x *termDB) audit(t *testing.T, ws string, want int) []db.AuditEntry {
	t.Helper()
	var rows []db.AuditEntry
	for range 100 {
		all, err := x.d.AllAudit(context.Background(), "", 500)
		if err != nil {
			t.Fatal(err)
		}
		rows = rows[:0]
		for _, e := range all {
			if e.WorkspaceID == ws && strings.HasPrefix(e.Action, "terminal.") {
				rows = append(rows, e)
			}
		}
		if len(rows) >= want {
			return rows
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("workspace %s: %d terminal audit rows, want %d: %+v", ws, len(rows), want, rows)
	return nil
}

func closeDetail(t *testing.T, rows []db.AuditEntry, reason string) {
	t.Helper()
	for _, e := range rows {
		if e.Action == "terminal.close" && strings.HasPrefix(e.Detail, "reason="+reason+" duration=") {
			return
		}
	}
	t.Fatalf("no terminal.close with reason=%s: %+v", reason, rows)
}

func TestTerminalSessionAuditsOpenAndClose(t *testing.T) {
	x := termDBRig(t)
	c, br := x.open(t, x.tn.ID, x.mine.ID)
	ready(t, br, true)
	c.Write(wsClientFrame(0x2, []byte("ls\r")))
	c.Write(wsClientFrame(0x1, []byte(`{"type":"resize","rows":40,"cols":120}`)))
	if f := <-x.hostd.frames; f.Kind != hostd.TermData || string(f.Data) != "ls\r" {
		t.Fatalf("data frame %+v", f)
	}
	if f := <-x.hostd.frames; f.Kind != hostd.TermResize || f.Rows != 40 || f.Cols != 120 {
		t.Fatalf("resize frame %+v", f)
	}
	c.Write(wsClientFrame(0x8, nil))
	if code, _ := closeFrame(t, br); code != 1000 {
		t.Fatalf("close code %d", code)
	}
	rows := x.audit(t, x.mine.ID, 2)
	opened := false
	for _, e := range rows {
		opened = opened || e.Action == "terminal.open" && e.Actor == "creator" && e.TenantID == x.tn.ID
	}
	if !opened {
		t.Fatalf("no terminal.open by the creator: %+v", rows)
	}
	closeDetail(t, rows, termClosed)
}

// Scoping: the terminal opens only for the workspace's creator or a tenant manager who is a
// member of that tenant; nothing is audited or dialed for a refusal.
func TestTerminalRefusesOtherWorkspacesAndTenants(t *testing.T) {
	x := termDBRig(t)
	cases := []struct {
		name, sub, role, tenant, ws string
		code                        int
	}{
		{"member, another member's workspace", "creator", "org_member", x.tn.ID, x.other.ID, 4403},
		{"another tenant's workspace under my tenant", "creator", "org_member", x.tn.ID, x.foreign.ID, 4404},
		{"another tenant's path", "creator", "org_member", x.tn2.ID, x.foreign.ID, 4404},
		{"no tenant at all", "creator", "", x.tn.ID, x.mine.ID, 4404},
	}
	for _, tc := range cases {
		x.as(tc.sub, tc.role)
		_, br := x.open(t, tc.tenant, tc.ws)
		if code, reason := closeFrame(t, br); code != tc.code || reason == "" {
			t.Errorf("%s: close %d %q, want %d", tc.name, code, reason, tc.code)
		}
	}
	// staff holding an approved tenant-wide grant are not members, so never allowed
	exp := time.Now().Add(time.Hour)
	c := &caller{id: &auth.Identity{Sub: "creator"}, op: "op_superadmin", managers: []string{"org_owner", "org_admin"},
		tenants: []tenantView{{Tenant: x.tn, grants: []db.Grant{{TenantID: x.tn.ID, Kind: "standard", Status: "approved", ExpiresAt: &exp}}}}}
	x.who.Store(c)
	_, br := x.open(t, x.tn.ID, x.mine.ID)
	if code, _ := closeFrame(t, br); code != 4403 {
		t.Errorf("staff with a grant: close %d, want 4403", code)
	}
	// a stopped workspace is refused before anything is dialed
	x.as("creator", "org_member")
	x.hostd.mu.Lock()
	x.hostd.status["w-"+x.mine.ID] = "Stopped"
	x.hostd.mu.Unlock()
	_, br = x.open(t, x.tn.ID, x.mine.ID)
	if code, _ := closeFrame(t, br); code != 4409 {
		t.Errorf("stopped workspace: close %d, want 4409", code)
	}
	if n := x.hostd.dials.Load(); n != 0 {
		t.Fatalf("hostd was dialed %d times for refused sessions", n)
	}
	for _, ws := range []string{x.mine.ID, x.other.ID, x.foreign.ID} {
		all, _ := x.d.AllAudit(context.Background(), "", 500)
		for _, e := range all {
			if e.WorkspaceID == ws && strings.HasPrefix(e.Action, "terminal.") {
				t.Fatalf("a refused session was audited as opened: %+v", e)
			}
		}
	}
	// a tenant admin who did not create the workspace may open it
	x.as("admin", "org_admin")
	x.hostd.mu.Lock()
	x.hostd.status["w-"+x.mine.ID] = "Running"
	x.hostd.mu.Unlock()
	cc, br := x.open(t, x.tn.ID, x.other.ID)
	ready(t, br, true)
	cc.Close()
	closeDetail(t, x.audit(t, x.other.ID, 2), termClosed)
}

// Mid-session: a caller who loses membership, a workspace that stops and a hostd that cannot
// be observed each end the session with their own reason within a re-check.
func TestTerminalRecheckEndsSession(t *testing.T) {
	x := termDBRig(t)
	useTermLimits(t, termLimits{idle: time.Hour, maxLife: time.Hour, recheck: 100 * time.Millisecond})
	for _, tc := range []struct {
		reason string
		code   int
		cause  func()
		undo   func()
	}{
		{termRevoked, 4003, func() { x.as("creator", "") }, func() { x.as("creator", "org_member") }},
		{termStopped, 4004, func() { x.setStatus(x.mine.ID, "Stopped") }, func() { x.setStatus(x.mine.ID, "Running") }},
		{termUnverified, 4006, func() { x.hostd.down.Store(true) }, func() { x.hostd.down.Store(false) }},
	} {
		_, br := x.open(t, x.tn.ID, x.mine.ID)
		ready(t, br, true)
		tc.cause()
		if code, reason := closeFrame(t, br); code != tc.code || reason != tc.reason {
			t.Fatalf("%s: close %d %q", tc.reason, code, reason)
		}
		tc.undo()
	}
	rows := x.audit(t, x.mine.ID, 6)
	for _, r := range []string{termRevoked, termStopped, termUnverified} {
		closeDetail(t, rows, r)
	}
}

func (x *termDB) setStatus(ws, st string) {
	x.hostd.mu.Lock()
	defer x.hostd.mu.Unlock()
	x.hostd.status["w-"+ws] = st
}

func TestTerminalIdleAndMaximum(t *testing.T) {
	x := termDBRig(t)
	useTermLimits(t, termLimits{idle: 200 * time.Millisecond, maxLife: time.Hour, recheck: time.Hour})
	_, br := x.open(t, x.tn.ID, x.mine.ID)
	ready(t, br, true)
	if code, reason := closeFrame(t, br); code != 4001 || reason != termIdle {
		t.Fatalf("idle: close %d %q", code, reason)
	}
	useTermLimits(t, termLimits{idle: time.Hour, maxLife: 200 * time.Millisecond, recheck: time.Hour})
	c, br := x.open(t, x.tn.ID, x.mine.ID)
	ready(t, br, true)
	go func() { // typing does not extend the maximum
		for range 20 {
			if _, err := c.Write(wsClientFrame(0x2, []byte("x"))); err != nil {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
	}()
	go func() {
		for range x.hostd.frames {
		}
	}()
	if code, reason := closeFrame(t, br); code != 4002 || reason != termMaximum {
		t.Fatalf("maximum: close %d %q", code, reason)
	}
	rows := x.audit(t, x.mine.ID, 4)
	closeDetail(t, rows, termIdle)
	closeDetail(t, rows, termMaximum)
}

func TestTerminalHostdRefusedAndLegacy(t *testing.T) {
	x := termDBRig(t)
	x.hostd.refuse = true
	_, br := x.open(t, x.tn.ID, x.mine.ID)
	if code, reason := closeFrame(t, br); code != 4502 || reason != "The terminal didn't open." {
		t.Fatalf("hostd refused: close %d %q", code, reason)
	}
	closeDetail(t, x.audit(t, x.mine.ID, 2), termUnavailable)
	// a hostd from before the framed stream: raw bytes, no resize
	x.hostd.refuse, x.hostd.legacy = false, true
	c, br := x.open(t, x.tn.ID, x.mine.ID)
	ready(t, br, false)
	c.Write(wsClientFrame(0x1, []byte(`{"type":"resize","rows":40,"cols":120}`)))
	c.Write(wsClientFrame(0x2, []byte("raw")))
	if got := <-x.hostd.raw; string(got) != "raw" {
		t.Fatalf("legacy hostd got %q", got)
	}
	c.Close()
	closeDetail(t, x.audit(t, x.mine.ID, 4), termClosed)
}
