package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(log.Writer())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/x/{id}", func(w http.ResponseWriter, r *http.Request) {
		noteCaller(r.Context(), "sub-123")
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {})
	h := RequestLog(mux)
	r := httptest.NewRequest("GET", "/v1/x/abc?token=SECRETVALUE", nil)
	r.Header.Set("Cookie", "s=COOKIEVALUE")
	h.ServeHTTP(httptest.NewRecorder(), r)
	out := buf.String()
	for _, want := range []string{"status=403", `route="GET /v1/x/{id}"`, "account=sub-123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "SECRETVALUE") || strings.Contains(out, "COOKIEVALUE") {
		t.Fatalf("secret leaked: %q", out)
	}
	buf.Reset()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/nope", nil))
	if strings.Contains(buf.String(), "/ok") || !strings.Contains(buf.String(), "status=404") {
		t.Fatalf("unexpected log: %q", buf.String())
	}
	var _ http.Hijacker = &statusWriter{}
}
