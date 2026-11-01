package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func base() map[string]string {
	return map[string]string{"GOLDSIGN_DIR": "/d", "GOLDSIGN_FACTS_DIR": "/f", "GOLDSIGN_OWNER_SUB": "sub-1",
		"GOLDSIGN_OWNER_NAME": "owner", "CONSOLE_PUBLIC_HOST": "console.example.test"}
}

func TestGoldSignStoreDerivesFromPublicHost(t *testing.T) {
	s := goldSignStore(envOf(base()))
	if s == nil || s.RPID != "console.example.test" || len(s.Origins) != 1 || s.Origins[0] != "https://console.example.test" {
		t.Fatalf("%+v", s)
	}
}

func TestGoldSignStoreOverridesAndRefusals(t *testing.T) {
	m := base()
	m["GOLDSIGN_ORIGINS"] = "https://a.example.test https://b.example.test"
	if s := goldSignStore(envOf(m)); s == nil || len(s.Origins) != 2 {
		t.Fatal("origins override ignored")
	}
	for _, bad := range []string{"https://console.example.test", "console.example.test:443", "192.0.2.1"} {
		m := base()
		m["CONSOLE_PUBLIC_HOST"] = bad
		if goldSignStore(envOf(m)) != nil {
			t.Fatalf("accepted host %q", bad)
		}
	}
	for _, k := range []string{"GOLDSIGN_OWNER_NAME", "GOLDSIGN_OWNER_SUB", "CONSOLE_PUBLIC_HOST"} {
		m := base()
		delete(m, k)
		if goldSignStore(envOf(m)) != nil {
			t.Fatalf("on without %s", k)
		}
	}
}

func TestParseNetsIsStrict(t *testing.T) {
	if n, err := parseNets(""); err != nil || len(n) != 0 {
		t.Fatal("empty must parse to no nets", n, err)
	}
	if n, err := parseNets("203.0.113.0/24 198.51.100.0/24"); err != nil || len(n) != 2 {
		t.Fatal(n, err)
	}
	for _, bad := range []string{"203.0.113.0/24 typo", "203.0.113.1", "203.0.113.0/33"} {
		if _, err := parseNets(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestSyncFailsClosedWithoutCIDRs(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	nets, _ := parseNets("203.0.113.0/24")
	for _, c := range []struct {
		nets []*net.IPNet
		addr string
		want int
	}{{nil, "203.0.113.53:5000", 401}, {nets, "203.0.113.53:5000", 204}, {nets, "192.0.2.5:5000", 401}} {
		h := &goldSignHTTP{syncToken: "t0ken", syncNets: c.nets}
		r := httptest.NewRequest("GET", "/v1/gold-sign/outbox", nil)
		r.RemoteAddr = c.addr
		w := httptest.NewRecorder()
		r.Header.Set("Authorization", "Bearer t0ken")
		h.gold(ok)(w, r)
		if w.Code != c.want {
			t.Fatalf("nets=%v addr=%s: got %d want %d", c.nets, c.addr, w.Code, c.want)
		}
	}
}
