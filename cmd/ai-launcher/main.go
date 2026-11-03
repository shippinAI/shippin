// ai-launcher runs inside a devspace: one click starts Codex CLI or Claude Code in a persistent
// tmux session, served to the browser (xterm.js) over a WebSocket. It listens on the address in
// its config (loopback or the workspace bridge); the edge's forward_auth is the only way in.
// usage: ai-launcher <config.json>
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shippin.cloud/console/internal/ailaunch"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: ai-launcher <config.json>")
	}
	cfg, err := ailaunch.Load(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	(&ailaunch.Handler{Cfg: cfg}).Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(cfg.Static, "launcher.html"))
	})
	mux.Handle("GET /assets/", http.FileServer(http.Dir(cfg.Static)))
	mux.Handle("GET /favicon.svg", http.FileServer(http.Dir(cfg.Static)))
	log.Printf("ai-launcher on %s (%d tools)", cfg.Addr, len(cfg.CLIs))
	srv := &http.Server{Addr: cfg.Addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
