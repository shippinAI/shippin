package hostd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidRepo(t *testing.T) {
	hosts := []string{"github.com", "gitlab.com"}
	for _, u := range []string{"https://github.com/vitejs/vite", "https://github.com/a/b.git", "https://gitlab.com/a-b/c_d.e/", "https://github.com/o/r.js"} {
		if ValidRepo(u, hosts) != nil {
			t.Errorf("%s rejected", u)
		}
	}
	for _, u := range []string{
		"http://github.com/a/b", "https://evil.com/a/b", "https://github.com.evil.com/a/b", "https://u:p@github.com/a/b",
		"https://github.com/a/..", "https://github.com/a/.", "https://github.com/../b", "https://github.com/a/-b",
		"https://github.com/a/b;rm -rf", "https://github.com/a/b?x=1", "https://github.com:22/a/b", "https://github.com/a/b/c",
		"https://github.com/a/$(id)", "https://github.com/a/.git", " https://github.com/a/b",
	} {
		if ValidRepo(u, hosts) == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

func TestValidTemplateRequest(t *testing.T) {
	s := testServer()
	ok := WorkspaceReq{Image: "shippin/tpl-node", Repo: "https://github.com/a/b", Size: Size{2, 4, 20}, Previews: []Preview{{Port: 5173}}}
	if err := s.validTemplate(ok); err != nil {
		t.Fatal(err)
	}
	for name, m := range map[string]func(*WorkspaceReq){
		"foreign image": func(r *WorkspaceReq) { r.Image = "images:debian/13" },
		"image flag":    func(r *WorkspaceReq) { r.Image = "shippin/--x y" },
		"repo":          func(r *WorkspaceReq) { r.Repo = "https://evil.com/a/b" },
		"cpu":           func(r *WorkspaceReq) { r.Size.CPU = 65 },
		"negative":      func(r *WorkspaceReq) { r.Size.DiskGiB = -1 },
		"startup":       func(r *WorkspaceReq) { r.Startup = strings.Repeat("x", 16<<10+1) },
		"dup preview":   func(r *WorkspaceReq) { r.Previews = []Preview{{Port: 5173}, {Port: 5173, Public: true}} },
		"ide preview":   func(r *WorkspaceReq) { r.Previews = []Preview{{Port: 3000}} },
		"low preview":   func(r *WorkspaceReq) { r.Previews = []Preview{{Port: 22}} },
	} {
		r := ok
		m(&r)
		if s.validTemplate(r) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestValidRule(t *testing.T) {
	s := testServer()
	if err := s.validRule(Rule{Protocol: "tcp", Port: 8080, Source: "203.0.113.0/24"}); err != nil {
		t.Fatal(err)
	}
	for _, r := range []Rule{{"tcp", 443, ""}, {"tcp", 80, ""}, {"tcp", 22, ""}, {"tcp", 22500, ""}, {"sctp", 8080, ""},
		{"tcp", 8080, "2001:db8::1"}, {"tcp", 8080, "1.2.3.4 } ; flush ruleset"}, {"tcp", 0, ""}} {
		if s.validRule(r) == nil {
			t.Errorf("%+v accepted", r)
		}
	}
}

func TestPreviewHostAndHostChecks(t *testing.T) {
	s := testServer()
	h := PreviewHost("dev-03e90d.ws.example.test", 5173)
	if h != "5173--dev-03e90d.ws.example.test" || strings.Count(strings.TrimSuffix(h, ".ws.example.test"), ".") != 0 {
		t.Fatalf("preview host must stay one label under the wildcard: %s", h)
	}
	if s.ValidHost("dev-03e90d.ws.example.test") != nil {
		t.Fatal("valid host rejected")
	}
	for _, h := range []string{"console.example.test", "a.b.ws.example.test", "ws.example.test", "x.ws.example.test.evil.com", "UP.ws.example.test"} {
		if s.ValidHost(h) == nil {
			t.Errorf("%q accepted as workspace host", h)
		}
	}
}

func TestReservedDomains(t *testing.T) {
	c := testServer().cfg
	for _, d := range []string{"example.test", "console.example.test", "x.ws.example.test"} {
		if !c.reservedDomain(d) {
			t.Errorf("%s not reserved", d)
		}
	}
	if c.reservedDomain("myexample.test") || c.reservedDomain("customer.com") {
		t.Error("suffix match must be on a label boundary")
	}
}

// proxyRoute must strip the console's workspace cookie before any upstream and (when private)
// put forward_auth before the upstream.
func TestProxyRouteStripsSessionCookie(t *testing.T) {
	s := testServer()
	for _, dev := range []bool{false, true} {
		b, _ := json.Marshal(s.proxyRoute("pv-x-5173", []string{"h"}, "10.128.1.3:5173", true, dev))
		js := string(b)
		if !strings.Contains(js, `"replace":{"Cookie"`) || !strings.Contains(js, `shippin_ws=[^;]*`) {
			t.Errorf("dev=%v: cookie not stripped: %s", dev, js)
		}
		if strings.Index(js, "/auth/ws-check") > strings.Index(js, "10.128.1.3:5173") {
			t.Errorf("dev=%v: forward_auth must run before the upstream", dev)
		}
	}
	pub, _ := json.Marshal(s.proxyRoute("pv-x-5173", []string{"h"}, "10.128.1.3:5173", false, true))
	if strings.Contains(string(pub), "ws-check") {
		t.Error("public preview must not require sign-in")
	}
}
