package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
)

// A backup is a pg_dump of the panel's database in the state directory. ClickHouse holds history only
// and is not part of it. Restoring needs the panel stopped, so it is a command, not a button.
var backupName = regexp.MustCompile(`^shippin-\d{8}-\d{6}\.dump$`)

// Backup is one recovery point.
type Backup struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) backupDir() string { return filepath.Join(h.StateDir, "backups") }

func (h *Handler) listBackups(w http.ResponseWriter, _ *http.Request, _ string) {
	items, err := h.backups()
	if err != nil {
		slog.Error("platform: backups not readable", "error", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The backups could not be read.")
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (h *Handler) backups() ([]Backup, error) {
	entries, err := os.ReadDir(h.backupDir())
	if os.IsNotExist(err) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Backup{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !backupName.MatchString(e.Name()) {
			continue
		}
		out = append(out, Backup{Name: e.Name(), Size: info.Size(), CreatedAt: info.ModTime().UTC()})
	}
	slices.SortFunc(out, func(a, b Backup) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

func (h *Handler) createBackup(w http.ResponseWriter, r *http.Request, _ string) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !(confirmation{Reason: in.Reason, Understood: true}).check(w) {
		return
	}
	if !h.Audit(w, r, "platform.backup.create", strings.TrimSpace(in.Reason)) {
		return
	}
	if err := os.MkdirAll(h.backupDir(), 0o700); err != nil {
		slog.Error("platform: backup directory not created", "error", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The backup directory could not be created.")
		return
	}
	name := "shippin-" + time.Now().UTC().Format("20060102-150405") + ".dump"
	final := filepath.Join(h.backupDir(), name)
	temp := final + ".partial"
	defer os.Remove(temp)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	if err := h.dump(ctx, temp); err != nil {
		slog.Error("platform: backup failed", "error", err)
		auth.Problem(w, http.StatusBadGateway, "The backup failed: "+err.Error())
		return
	}
	entries, err := backupEntries(ctx, temp)
	if err != nil || entries == 0 {
		slog.Error("platform: backup did not verify", "entries", entries, "error", err)
		auth.Problem(w, http.StatusBadGateway, "The backup was written but could not be read back, so it was discarded.")
		return
	}
	if err := os.Rename(temp, final); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The backup could not be saved.")
		return
	}
	info, _ := os.Stat(final)
	writeJSON(w, map[string]any{"backup": Backup{Name: name, Size: info.Size(), CreatedAt: info.ModTime().UTC()}, "entries": entries})
}

// dump runs pg_dump with the connection in its environment, never on a command line.
func (h *Handler) dump(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--file="+path)
	env, err := pgEnv(h.DatabaseURL)
	if err != nil {
		return err
	}
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", firstLine(out, err))
	}
	return os.Chmod(path, 0o600)
}

func backupEntries(ctx context.Context, path string) (int, error) {
	out, err := exec.CommandContext(ctx, "pg_restore", "--list", path).Output()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" && !strings.HasPrefix(line, ";") {
			n++
		}
	}
	return n, nil
}

func firstLine(out []byte, err error) string {
	if s := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]); s != "" {
		return s
	}
	return err.Error()
}

func pgEnv(raw string) ([]string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errNoDatabase
	}
	password, _ := u.User.Password()
	env := []string{"PGHOST=" + u.Hostname(), "PGUSER=" + u.User.Username(), "PGPASSWORD=" + password, "PGDATABASE=" + strings.TrimPrefix(u.Path, "/")}
	if p := u.Port(); p != "" {
		env = append(env, "PGPORT="+p)
	}
	if m := u.Query().Get("sslmode"); m != "" {
		env = append(env, "PGSSLMODE="+m)
	}
	return env, nil
}

func (h *Handler) backupPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if !backupName.MatchString(name) {
		auth.Problem(w, http.StatusNotFound, "There is no backup with that name.")
		return "", false
	}
	path := filepath.Join(h.backupDir(), name)
	if _, err := os.Stat(path); err != nil {
		auth.Problem(w, http.StatusNotFound, "There is no backup with that name.")
		return "", false
	}
	return path, true
}

func (h *Handler) downloadBackup(w http.ResponseWriter, r *http.Request, _ string) {
	path, ok := h.backupPath(w, r)
	if !ok || !h.Audit(w, r, "platform.backup.download", r.PathValue("name")) {
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("name")+`"`)
	http.ServeFile(w, r, path)
}

func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request, _ string) {
	path, ok := h.backupPath(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("confirm_name") != r.PathValue("name") {
		auth.Problem(w, http.StatusBadRequest, "Confirm the name of the backup you are deleting.")
		return
	}
	if !h.Audit(w, r, "platform.backup.delete", r.PathValue("name")) {
		return
	}
	if err := os.Remove(path); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The backup could not be deleted.")
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

// revealRecoveryKey hands over the key that seals saved secrets. A restored database cannot open those
// secrets without it, so disaster recovery needs both.
func (h *Handler) revealRecoveryKey(w http.ResponseWriter, r *http.Request, _ string) {
	var in confirmation
	if !decode(w, r, &in) || !in.check(w) {
		return
	}
	if !h.Audit(w, r, "platform.recovery_key.reveal", strings.TrimSpace(in.Reason)) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]string{"key": h.SessionKey})
}
