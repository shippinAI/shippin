package hv

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

const untrustedIncus = `{"type":"sync","status":"Success","status_code":200,"metadata":{"api_extensions":["etag"],"api_status":"stable","api_version":"1.0","auth":"untrusted","public":false,"auth_methods":["tls"]}}`

// fake is a TLS server that answers like one kind, and counts anything a probe must never send.
type fake struct {
	*httptest.Server
	leaked atomic.Int32
}

func newFake(t *testing.T, tls bool, h http.HandlerFunc) *fake {
	t.Helper()
	f := &fake{}
	wrapped := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.ContentLength > 0 {
			f.leaked.Add(1)
		}
		h(w, r)
	}
	if tls {
		f.Server = httptest.NewTLSServer(http.HandlerFunc(wrapped))
	} else {
		f.Server = httptest.NewServer(http.HandlerFunc(wrapped))
	}
	t.Cleanup(f.Close)
	return f
}

func incusHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/1.0" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(untrustedIncus))
}

func proxmoxHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", "pve-api-daemon/3.0")
	if r.URL.Path == "/api2/json/version" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusNotImplemented)
}

func hostPort(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestDetectRecognisesEachKindOnAGivenPort(t *testing.T) {
	incus := newFake(t, true, incusHandler)
	pve := newFake(t, true, proxmoxHandler)
	versioned := newFake(t, true, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/version" {
			_, _ = w.Write([]byte(`{"data":{"version":"9.0.11","release":"9.0","repoid":"abc"}}`))
			return
		}
		http.NotFound(w, r)
	})
	for _, c := range []struct {
		name    string
		srv     *fake
		kind    string
		version string
		saw     string
	}{
		{"incus", incus, "incus", "", "answered /1.0 as Incus, untrusted"},
		{"proxmox asking for a token", pve, "proxmox", "", "401 from pve-api-daemon"},
		{"proxmox with its version", versioned, "proxmox", "9.0.11", "as Proxmox VE 9.0.11"},
	} {
		t.Run(c.name, func(t *testing.T) {
			addr := hostPort(t, c.srv.URL)
			d, err := Detect(context.Background(), addr)
			if err != nil {
				t.Fatal(err)
			}
			if d.Outcome != DetectRecognised || d.Kind != c.kind || d.Version != c.version || d.BaseURL != "https://"+addr || d.Address != addr {
				t.Fatalf("%+v", d)
			}
			if len(d.Probes) != 1 {
				t.Fatalf("a given port is the only one probed: %+v", d.Probes)
			}
			if d.Certificate == nil || !strings.Contains(d.Certificate.Fingerprint, ":") || d.Certificate.PEM == "" || d.CertificateVerified {
				t.Fatalf("the unknown certificate is shown, not trusted: %+v", d.Certificate)
			}
			if !slices.ContainsFunc(d.Evidence, func(e string) bool { return strings.Contains(e, c.saw) }) {
				t.Fatalf("evidence %q lacks %q", d.Evidence, c.saw)
			}
			if c.srv.leaked.Load() != 0 {
				t.Fatal("a probe sent credentials, a body or a method other than GET")
			}
		})
	}
}

func TestDetectTriesEveryDefaultPortWhenNoneIsGiven(t *testing.T) {
	incus := newFake(t, true, incusHandler)
	pve := newFake(t, true, proxmoxHandler)
	ip, incusPort, _ := net.SplitHostPort(hostPort(t, incus.URL))
	_, pvePort, _ := net.SplitHostPort(hostPort(t, pve.URL))
	rs := []recogniser{recognisers[0], recognisers[1]}
	rs[0].port, rs[1].port = incusPort, pvePort
	d, err := detect(context.Background(), ip, rs)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Probes) != 2 || d.Address != ip {
		t.Fatalf("both default ports are probed: %+v", d)
	}
	if d.Kind != "incus" || d.Probes[1].Kind != "proxmox" {
		t.Fatalf("the first kind found leads; the other is listed: %+v", d)
	}
	if !slices.ContainsFunc(d.Evidence, func(e string) bool { return strings.Contains(e, "also answered, as proxmox") }) {
		t.Fatalf("a second finding is pointed out: %q", d.Evidence)
	}
	// Each probe says why the other kind did not match there.
	if !slices.ContainsFunc(d.Probes[0].Evidence, func(e string) bool { return strings.Contains(e, "/api2/json/version answered HTTP 404") }) ||
		!slices.ContainsFunc(d.Probes[1].Evidence, func(e string) bool { return strings.Contains(e, "/1.0 answered HTTP 501") }) {
		t.Fatalf("%q / %q", d.Probes[0].Evidence, d.Probes[1].Evidence)
	}
}

func TestDetectTellsUnreachableFromUnrecognised(t *testing.T) {
	neither := newFake(t, true, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>hello</html>")) })
	plain := newFake(t, false, incusHandler)
	redirect := newFake(t, true, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/1.0", http.StatusFound)
	})
	lxd := newFake(t, true, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"metadata":{"api_version":"1.0","auth":"trusted","environment":{"server":"lxd","server_version":"5.21"}}}`))
	})
	impostor := newFake(t, true, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := l.Addr().String()
	_ = l.Close()

	for _, c := range []struct {
		name, addr string
		outcome    DetectOutcome
		cert       bool
		saw        string
	}{
		{"https that is neither", hostPort(t, neither.URL), DetectUnrecognised, true, "/1.0 answered, but not with the Incus API"},
		{"plain http", hostPort(t, plain.URL), DetectUnrecognised, false, "does not speak TLS"},
		{"a redirect is not followed", hostPort(t, redirect.URL), DetectUnrecognised, true, "/1.0 answered HTTP 302"},
		{"lxd is not incus", hostPort(t, lxd.URL), DetectUnrecognised, true, `as "lxd"`},
		{"401 from something else", hostPort(t, impostor.URL), DetectUnrecognised, true, "but not as Proxmox VE"},
		{"closed port", closed, DetectUnreachable, false, "connection refused"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, err := Detect(context.Background(), c.addr)
			if err != nil {
				t.Fatal(err)
			}
			if d.Outcome != c.outcome || d.Kind != "" || (d.Certificate != nil) != c.cert {
				t.Fatalf("%+v", d)
			}
			if !slices.ContainsFunc(d.Evidence, func(e string) bool { return strings.Contains(e, c.saw) }) {
				t.Fatalf("evidence %q lacks %q", d.Evidence, c.saw)
			}
		})
	}
}

func TestDetectRefusesAddressesItWillNotTry(t *testing.T) {
	for _, bad := range []string{
		"", "   ", "http://host", "ftp://host", "file:///etc/passwd", "https://user:pw@host", "user@host",
		"https://host/api2/json", "https://host?x=1", "https://host#frag", "host/path", "host:0", "host:65536", "host:x",
		"host:", "https://host:", "bad_name", "-host", "fe80::1%eth0", "host name", strings.Repeat("a", 301),
	} {
		if _, err := Detect(context.Background(), bad); !errors.Is(err, ErrBadAddress) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	for in, want := range map[string][2]string{
		"pve.example":               {"pve.example", ""},
		" pve.example:8006 ":        {"pve.example", "8006"},
		"https://pve.example":       {"pve.example", ""},
		"HTTPS://pve.example:8006/": {"pve.example", "8006"},
		"10.0.0.5":                  {"10.0.0.5", ""},
		"[2001:db8::1]:8443":        {"2001:db8::1", "8443"},
		"2001:db8::1":               {"2001:db8::1", ""},
		"[2001:db8::1]":             {"2001:db8::1", ""},
		"https://[2001:db8::1]":     {"2001:db8::1", ""},
	} {
		host, port, err := ParseAddress(in)
		if err != nil || host != want[0] || port != want[1] {
			t.Errorf("%q: %q %q %v", in, host, port, err)
		}
	}
}

func TestEveryRecogniserIsADriver(t *testing.T) {
	for _, r := range recognisers {
		if _, ok := SpecFor(r.kind); !ok {
			t.Errorf("%s is detected but is not in Specs", r.kind)
		}
	}
	if !slices.Equal(DetectOutcome("").Values(), []string{"recognised", "unrecognised", "unreachable"}) {
		t.Error(DetectOutcome("").Values())
	}
}
