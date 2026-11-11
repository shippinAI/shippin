package api

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// RequestLog (t1029) logs every 4xx/5xx response: route pattern, status, duration and the
// caller's IdP subject when one was resolved. Concrete path values (ids, device codes) are never
// logged (T-6): matched requests log only the pattern, unmatched ones a redacted path. No
// headers, cookies, query strings or bodies are logged.
func RequestLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		info := &reqInfo{}
		r = r.WithContext(context.WithValue(r.Context(), reqInfoKey{}, info))
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(sw, r)
		if sw.status < 400 || sw.hijacked && sw.status == http.StatusOK {
			return
		}
		route := r.Pattern
		if route == "" {
			route = r.Method + " " + redactPath(r.URL.Path)
		}
		acct := info.sub
		if acct == "" {
			acct = "-"
		}
		level := slog.LevelWarn
		if sw.status >= 500 {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "http request", "status", sw.status, "route", route, "account", acct, "dur_ms", time.Since(start).Milliseconds(), "request_id", id)
	})
}

type reqInfoKey struct{}

type reqInfo struct{ sub string }

// noteCaller records the resolved subject for RequestLog (no-op outside it).
func noteCaller(ctx context.Context, sub string) {
	if i, ok := ctx.Value(reqInfoKey{}).(*reqInfo); ok {
		i.sub = sub
	}
}

type statusWriter struct {
	http.ResponseWriter
	status   int
	wrote    bool
	hijacked bool
}

func (s *statusWriter) WriteHeader(c int) {
	if !s.wrote {
		s.status, s.wrote = c, true
	}
	s.ResponseWriter.WriteHeader(c)
}

func (s *statusWriter) Write(p []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(p)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack keeps WebSockets (terminal, bridge) working through the wrapper.
func (s *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack unsupported")
	}
	s.hijacked = true
	return hj.Hijack()
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
