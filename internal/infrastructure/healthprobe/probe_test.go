package healthprobe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestProbeTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	got := probe(context.Background(), Target{Address: net.ParseIP("127.0.0.1"), Port: port, Kind: "tcp", Timeout: time.Second})
	if got.Status != "passing" || got.Duration <= 0 {
		t.Fatalf("TCP result = %#v", got)
	}
	got = probe(context.Background(), Target{Address: net.ParseIP("127.0.0.1"), Port: port + 1, Kind: "tcp", Timeout: time.Second})
	if got.Status != "failing" || got.Failure != "application_probe_failed" {
		t.Fatalf("closed TCP result = %#v", got)
	}
}

func TestProbeHTTPRequires2xxAndDoesNotFollowRedirects(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/health", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	base := Target{Address: net.ParseIP(u.Hostname()), Port: port, Kind: "http", Timeout: time.Second}
	base.Path = "/health"
	if got := probe(context.Background(), base); got.Status != "passing" {
		t.Fatalf("2xx result = %#v", got)
	}
	base.Path = "/redirect"
	if got := probe(context.Background(), base); got.Status != "failing" || got.Failure != "application_probe_failed" {
		t.Fatalf("redirect result = %#v", got)
	}
	if requests != 2 {
		t.Fatalf("redirect was followed; got %d requests", requests)
	}
}

func TestValidateRejectsUntrustedTargetsAndMalformedChecks(t *testing.T) {
	cases := []Target{
		{Address: net.ParseIP("127.0.0.1"), Port: 5432, Kind: "tcp", Timeout: time.Second},
		{Address: net.ParseIP("8.8.8.8"), Port: 5432, Kind: "tcp", Timeout: time.Second},
		{Address: net.ParseIP("10.0.0.2"), Port: 0, Kind: "tcp", Timeout: time.Second},
		{Address: net.ParseIP("10.0.0.2"), Port: 5432, Kind: "http", Path: "//evil", Timeout: time.Second},
		{Address: net.ParseIP("10.0.0.2"), Port: 5432, Kind: "http", Path: "/health?x=1", Timeout: time.Second},
	}
	for i, target := range cases {
		if err := validate(target); err == nil {
			t.Errorf("case %d unexpectedly valid", i)
		}
	}
	if err := validate(Target{Address: net.ParseIP("10.0.0.2"), Port: 8080, Kind: "http", Path: "/health", Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
}
