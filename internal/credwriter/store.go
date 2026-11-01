// Package credwriter is the single per-workspace credential writer: it ingests token requests
// hostd spools in, persists them on the AI home (so they survive restarts), keeps the env file
// login shells source, runs the vendors' refresh commands and reports status. It never logs or
// prints a credential.
package credwriter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"shippin.cloud/console/internal/hostd"
)

// Saved is one stored token on the AI home (mode 0600).
type Saved struct {
	Token     string     `json:"token"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	SavedAt   time.Time  `json:"saved_at"`
}

type Writer struct {
	Cfg hostd.WriterConfig
	// Run executes a vendor argv with optional stdin and reports success (exec seam for tests).
	Run func(argv []string, stdin string) bool
	Now func() time.Time
}

func (w *Writer) savedPath(p string) string { return filepath.Join(w.Cfg.StoreDir, p+".token.json") }

func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (w *Writer) load(p string) *Saved {
	b, err := os.ReadFile(w.savedPath(p))
	if err != nil {
		return nil
	}
	var s Saved
	if json.Unmarshal(b, &s) != nil || s.Token == "" {
		return nil
	}
	return &s
}

// Ingest applies and removes every spooled request.
func (w *Writer) Ingest() {
	files, _ := filepath.Glob(filepath.Join(w.Cfg.SpoolDir, "*.json"))
	for _, f := range files {
		p := strings.TrimSuffix(filepath.Base(f), ".json")
		prov, ok := w.Cfg.Providers[p]
		b, err := os.ReadFile(f)
		_ = os.Remove(f)
		var req hostd.SpoolReq
		if !ok || err != nil || json.Unmarshal(b, &req) != nil {
			continue
		}
		switch req.Op {
		case "set":
			s, _ := json.Marshal(Saved{Token: req.Token, ExpiresAt: req.ExpiresAt, SavedAt: w.Now().UTC()})
			if writeFile(w.savedPath(p), s) == nil && len(prov.TokenLogin) > 0 {
				w.Run(prov.TokenLogin, req.Token+"\n")
			}
		case "forget":
			_ = os.Remove(w.savedPath(p))
			w.Run(prov.Logout, "")
		}
	}
}

// shellQuote single-quotes v for the sourced env file.
func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

// WriteEnv rewrites the env file login shells source: CLI homes plus stored tokens.
func (w *Writer) WriteEnv() error {
	var lines []string
	for k, v := range w.Cfg.Env {
		lines = append(lines, "export "+k+"="+shellQuote(v))
	}
	for id, p := range w.Cfg.Providers {
		if s := w.load(id); s != nil && !w.expired(s) {
			lines = append(lines, "export "+p.TokenEnv+"="+shellQuote(s.Token))
		}
	}
	sort.Strings(lines)
	return writeFile(filepath.Join(w.Cfg.StoreDir, "env"), []byte(strings.Join(lines, "\n")+"\n"))
}

func (w *Writer) expired(s *Saved) bool { return s.ExpiresAt != nil && !s.ExpiresAt.After(w.Now()) }
