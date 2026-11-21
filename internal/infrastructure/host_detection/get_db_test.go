package host_detection_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/infrastructure/host_detection"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const route = "/v1/operator/infrastructure/host-detection"

func serve(t *testing.T, withProber bool) *mcpltest.Harness {
	s := mcpl.Services{}
	if withProber {
		host_detection.Services(s)
	}
	return mcpltest.Serve(t, actions.All, "shippin.infrastructure.host_detection.", mcpltest.As{Actor: "ada", Services: s})
}

func tlsServer(t *testing.T, h http.HandlerFunc) string {
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u.Host
}

func ask(address string) string { return route + "?address=" + url.QueryEscape(address) }

// TestGet runs the detection on its route against a fresh database, with the prober that really connects,
// against local servers that answer as each kind, as neither, and not at all.
func TestGet(t *testing.T) {
	h := serve(t, true)
	incus := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1.0" {
			_, _ = w.Write([]byte(`{"metadata":{"api_version":"1.0","auth":"untrusted"}}`))
			return
		}
		http.NotFound(w, r)
	})
	pve := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "pve-api-daemon/3.0")
		w.WriteHeader(http.StatusUnauthorized)
	})
	neither := tlsServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) })
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := l.Addr().String()
	_ = l.Close()

	for _, c := range []struct{ address, outcome, kind string }{
		{incus, "recognised", "incus"},
		{"https://" + pve + "/", "recognised", "proxmox"},
		{neither, "unrecognised", ""},
		{closed, "unreachable", ""},
	} {
		got := h.Call("GET", ask(c.address), "", 200)
		kind, _ := got["kind"].(string)
		if got["outcome"] != c.outcome || kind != c.kind {
			t.Fatalf("%s: %v", c.address, got)
		}
		if c.outcome == "recognised" {
			cert, _ := got["certificate"].(map[string]any)
			if got["base_url"] == "" || cert == nil || !strings.Contains(cert["fingerprint"].(string), ":") || got["certificate_verified"] != false {
				t.Fatalf("%s: the base URL and the unknown certificate are shown: %v", c.address, got)
			}
		}
		if ev, _ := got["evidence"].([]any); len(ev) == 0 {
			t.Fatalf("%s: no evidence: %v", c.address, got)
		}
	}

	// Refusals: an address it will not try is a 400 with a plain sentence, before anything is dialled.
	for bad, says := range map[string]string{
		"":                       "Enter the server's address",
		"ftp://" + incus:         "Only an https address",
		"https://root:pw@" + pve: "Leave the user name",
		"https://" + pve + "/x":  "Leave the path out",
		"https://" + pve + "?a":  "Leave the query out",
		"host:99999":             "The port must be a number",
	} {
		got := h.Call("GET", ask(bad), "", 400)
		if msg, _ := got["message"].(string); !strings.HasPrefix(msg, says) || !strings.HasSuffix(msg, ".") {
			t.Errorf("%q: %v", bad, got)
		}
	}
	h.Call("GET", route, "", 400)

	// A read: no reason, no audit entry, no event.
	if len(h.Audits) != 0 || len(h.Events) != 0 {
		t.Fatalf("a detection changed something: %v %v", h.Audits, h.Events)
	}
	// Without the prober the install answers 503, not a guess.
	serve(t, false).Call("GET", ask(incus), "", 503)
}

// The gate is the operator route's permission (internal/api decides it): adding hosts, as a read.
func TestGetIsAReadForWhoeverMayAddHosts(t *testing.T) {
	i := slices.IndexFunc(actions.All, func(g mcpl.Registered) bool { return g.ID == "shippin.infrastructure.host_detection.get" })
	if i < 0 {
		t.Fatal("not registered")
	}
	a := actions.All[i].Action
	if a.Route != "GET "+route || a.Access != mcpl.Staff || a.Perm != "infra.hosts.write" || a.Risk != mcpl.Read || a.Reason || a.Event != "" {
		t.Fatalf("%+v", a)
	}
}
