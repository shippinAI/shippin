package incus

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
)

// The Incus on the panel's own machine is reached through its Unix socket, with no TLS or certificate.
func TestHealthOverTheLocalSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "unix.socket")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets are not available here: %v", err)
	}
	var got string
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
		_, _ = w.Write([]byte(`{"type":"sync","metadata":{"auth":"trusted","environment":{"server_version":"6.0"}}}`))
	})}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	c, err := New(Endpoint{BaseURL: "unix://" + filepath.ToSlash(sock)}, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Health(context.Background())
	if err != nil || !h.Online || got != "/1.0" {
		t.Fatalf("health %+v %v, asked %q", h, err, got)
	}
}

func TestSocketPath(t *testing.T) {
	for in, want := range map[string]string{"unix:///var/lib/incus/unix.socket": "/var/lib/incus/unix.socket", "unix://relative": "", "https://h:8443": "", "": ""} {
		if got := SocketPath(in); got != want {
			t.Errorf("SocketPath(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := New(Endpoint{BaseURL: "unix:relative"}, func() (string, error) { return "", nil }); err == nil {
		t.Error("a socket address without an absolute path was accepted")
	}
}
