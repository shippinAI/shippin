package stateapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/rrflow/state"
)

// Server is the rrflow/state HTTP API. Every request is scoped to the caller's bound workspace:
// a workspace sees and writes only its own sessions, memory and decisions.
type Server struct {
	Stores   *state.Namespaces // one store per project namespace (D36 shared instance)
	Bindings *Bindings
}

type storeKey struct{}

// store is the calling binding's namespace store (set by auth).
func store(r *http.Request) *state.Store { return r.Context().Value(storeKey{}).(*state.Store) }

type ctxFn func(w http.ResponseWriter, r *http.Request, ws string)

// Routes is the closed set of endpoints (02c rrflow/state: sessions, memory, decision log).
func (s *Server) Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, map[string]string{"status": "ok", "service": "rrflow-state"})
	})
	m.HandleFunc("POST /v1/sessions", s.auth(s.startSession))
	m.HandleFunc("POST /v1/sessions/{id}/end", s.auth(s.endSession))
	m.HandleFunc("GET /v1/sessions", s.auth(func(w http.ResponseWriter, r *http.Request, ws string) {
		reply(w, 200, map[string]any{"items": store(r).Sessions(ws, limit(r))})
	}))
	m.HandleFunc("GET /v1/memory", s.auth(func(w http.ResponseWriter, r *http.Request, ws string) {
		reply(w, 200, map[string]any{"items": store(r).Memory(scope(ws), limit(r))})
	}))
	m.HandleFunc("GET /v1/memory/{key...}", s.auth(s.getMemory))
	m.HandleFunc("PUT /v1/memory/{key...}", s.auth(s.putMemory))
	m.HandleFunc("DELETE /v1/memory/{key...}", s.auth(s.deleteMemory))
	m.HandleFunc("POST /v1/decisions", s.auth(s.addDecision))
	m.HandleFunc("GET /v1/decisions", s.auth(func(w http.ResponseWriter, r *http.Request, ws string) {
		reply(w, 200, map[string]any{"items": store(r).Decisions(ws, limit(r))})
	}))
	return m
}

// scope is the MVP memory scope (02c "Memory is per workspace in the MVP").
func scope(ws string) string { return "workspace:" + ws }

func (s *Server) auth(h ctxFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		b, found := Binding{}, false
		if ok {
			b, found = s.Bindings.Authenticate(strings.TrimSpace(tok))
		}
		if !found {
			reply(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		st, err := s.Stores.Get(b.Namespace)
		if err != nil {
			fail(w, err)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), storeKey{}, st))
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		h(w, r, b.Workspace)
	}
}

func limit(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n < 1 || n > 500 {
		return 100
	}
	return n
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// fail maps store errors to HTTP codes without echoing input back.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, state.ErrInvalid):
		reply(w, http.StatusBadRequest, map[string]string{"error": "invalid input"})
	case errors.Is(err, state.ErrNotFound):
		reply(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		reply(w, http.StatusInternalServerError, map[string]string{"error": "storage error"})
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if json.NewDecoder(r.Body).Decode(v) != nil {
		reply(w, http.StatusBadRequest, map[string]string{"error": "bad body"})
		return false
	}
	return true
}
