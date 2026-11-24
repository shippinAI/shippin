package main

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/runtime/runfleet"
)

func TestServerClosesIncompleteRequestHeaders(t *testing.T) {
	svc, err := runfleet.Open(t.TempDir(), runfleet.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(svc, serverConfig{Token: "test-token"})
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Fatal("HTTP server must bound header, body, response and idle lifetimes")
	}
	srv.ReadHeaderTimeout = 50 * time.Millisecond
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close(); <-done })
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("GET /healthz HTTP/1.1\r\nHost: example.test\r\n")); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		t.Fatal("server left incomplete headers open until the client deadline")
	}
	if strings.Contains(line, "200 OK") {
		t.Fatal("server accepted incomplete request headers")
	}
}
