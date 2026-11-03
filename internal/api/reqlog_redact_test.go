package api

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedactPath(t *testing.T) {
	for in, want := range map[string]string{
		"/v1/bridge/device/ABCD-1234":      "/v1/bridge/device/:x",
		"/v1/tenants/01a14149-df75-780b/x": "/v1/tenants/:x/x",
		"/auth/login":                      "/auth/login",
		"/v1/me/ssh-keys":                  "/v1/me/ssh-keys",
		"/t/abc123def":                     "/t/:x",
		"/assets/index-zhwoPRE_.js":        "/assets/:x",
		"/v1/x/" + strings.Repeat("a", 30): "/v1/x/:x",
	} {
		if got := redactPath(in); got != want {
			t.Errorf("redactPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// T-6: a 4xx on a matched route logs the pattern only, never the concrete path value.
func TestRequestLogNoPathValues(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/bridge/device/{code}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	h := RequestLog(mux)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/bridge/device/PAIR-CODE-77", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/nope/SECRET-SEG-99", nil))
	out := buf.String()
	if strings.Contains(out, "PAIR-CODE-77") || strings.Contains(out, "SECRET-SEG-99") || strings.Contains(out, "path=") {
		t.Fatalf("path value leaked: %q", out)
	}
	if !strings.Contains(out, `route="GET /v1/bridge/device/{code}"`) || !strings.Contains(out, `route="GET /v1/nope/:x"`) {
		t.Fatalf("missing routes: %q", out)
	}
}

// A4 follow-up: a real hijack (101 + raw bytes) through RequestLog works and is not logged.
func TestRequestLogHijackRuntime(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		c, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer c.Close()
		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\nhello")
		rw.Flush()
	})
	srv := httptest.NewServer(RequestLog(mux))
	defer srv.Close()
	c, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(c, "GET /ws HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	br := bufio.NewReader(c)
	res, err := http.ReadResponse(br, nil)
	if err != nil || res.StatusCode != 101 {
		t.Fatalf("want 101, got %v %v", res, err)
	}
	b := make([]byte, 5)
	if _, err := io.ReadFull(br, b); err != nil || string(b) != "hello" {
		t.Fatalf("bytes after upgrade: %q %v", b, err)
	}
	if strings.Contains(buf.String(), "/ws") {
		t.Fatalf("hijacked request logged: %q", buf.String())
	}
}
