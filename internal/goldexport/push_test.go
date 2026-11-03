package goldexport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCheckURL(t *testing.T) {
	for u, ok := range map[string]bool{"https://console.example.invalid": true, "http://192.0.2.53:8080": false, "https://192.0.2.53:8443": true, "https://": false,
		"http://192.0.2.10": false, "http://console.example.invalid": false, "ftp://192.0.2.53": false} {
		if (CheckURL(u) == nil) != ok {
			t.Errorf("%s: want ok=%v", u, ok)
		}
	}
}

func TestPull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[{"kid":"a"},{"kid":"b"}]`)) }))
	defer srv.Close()
	in := t.TempDir()
	n, err := (&Client{Base: srv.URL, Token: "t", HTTP: srv.Client()}).Pull(context.Background(), in)
	files, _ := filepath.Glob(filepath.Join(in, "*.json"))
	if err != nil || n != 2 || len(files) != 2 {
		t.Fatal(n, err, files)
	}
}
