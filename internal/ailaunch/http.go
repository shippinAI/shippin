package ailaunch

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
)

// Handler serves /v1/ai/*: list, launch, stop and attach (WebSocket).
type Handler struct{ Cfg *Config }

func (h *Handler) Register(m *http.ServeMux) {
	m.HandleFunc("GET /v1/ai/sessions", h.list)
	m.HandleFunc("POST /v1/ai/sessions/{id}", h.launch)
	m.HandleFunc("DELETE /v1/ai/sessions/{id}", h.stop)
	m.HandleFunc("GET /v1/ai/sessions/{id}/attach", h.attach)
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func dims(r *http.Request) (uint16, uint16) {
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	return uint16(min(max(rows, 10), 200)), uint16(min(max(cols, 40), 400))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	out := []Session{}
	for i := range h.Cfg.CLIs {
		out = append(out, h.Cfg.Status(r.Context(), &h.Cfg.CLIs[i]))
	}
	reply(w, http.StatusOK, map[string]any{"sessions": out})
}

func (h *Handler) find(w http.ResponseWriter, r *http.Request) *CLI {
	x := h.Cfg.cli(r.PathValue("id"))
	if x == nil {
		reply(w, http.StatusNotFound, map[string]string{"message": "Unknown tool."})
	}
	return x
}

func (h *Handler) launch(w http.ResponseWriter, r *http.Request) {
	if x := h.find(w, r); x != nil {
		rows, cols := dims(r)
		s, err := h.Cfg.Ensure(r.Context(), x, rows, cols)
		if err != nil {
			log.Printf("ailaunch: start %s: %v", x.ID, err)
			reply(w, http.StatusInternalServerError, map[string]string{"message": "The tool could not start."})
			return
		}
		reply(w, http.StatusOK, s)
	}
}

func (h *Handler) stop(w http.ResponseWriter, r *http.Request) {
	if x := h.find(w, r); x != nil {
		_ = h.Cfg.Stop(r.Context(), x)
		w.WriteHeader(http.StatusNoContent)
	}
}

// attach bridges one WebSocket to a fresh `tmux attach` on a PTY. Binary frames are keystrokes;
// text frames are {"rows":n,"cols":n} resizes. Closing either side detaches only.
func (h *Handler) attach(w http.ResponseWriter, r *http.Request) {
	x := h.find(w, r)
	if x == nil {
		return
	}
	if !h.Cfg.Status(r.Context(), x).Running {
		reply(w, http.StatusConflict, map[string]string{"message": "Not running: launch it first."})
		return
	}
	conn, rw, err := wsAccept(w, r, h.Cfg.Origins)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	defer conn.Close()
	rows, cols := dims(r)
	cmd := h.Cfg.attachCmd(x)
	cmd.Env = append(cmd.Environ(), "TERM=xterm-256color")
	pty, err := startPTY(cmd, rows, cols)
	if err != nil {
		log.Printf("ailaunch: attach %s: %v", x.ID, err)
		return
	}
	var once sync.Once
	done := func() { once.Do(func() { _ = cmd.Process.Kill(); _ = pty.Close(); _ = conn.Close() }) }
	defer func() { done(); _ = cmd.Wait() }()
	var mu sync.Mutex
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := pty.Read(buf)
			if err != nil {
				done()
				return
			}
			mu.Lock()
			err = wsWrite(conn, 0x2, buf[:n])
			mu.Unlock()
			if err != nil {
				done()
				return
			}
		}
	}()
	for {
		op, p, err := wsRead(rw.Reader, lockedWriter{conn, &mu})
		if err != nil {
			return
		}
		if op == 0x1 {
			var z struct{ Rows, Cols uint16 }
			if json.Unmarshal(p, &z) == nil && z.Rows > 0 && z.Cols > 0 {
				_ = resize(pty, z.Rows, z.Cols)
			}
			continue
		}
		if _, err := pty.Write(p); err != nil {
			return
		}
	}
}
