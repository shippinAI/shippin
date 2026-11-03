package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/version"
)

// updateStatus is what the last check found. Checked stays false until one has finished, so the
// panel never says "up to date" about something it has not looked at.
type updateStatus struct {
	Checked    bool      `json:"checked"`
	CheckedAt  time.Time `json:"checked_at,omitzero"`
	Available  bool      `json:"available"`
	Behind     int       `json:"behind"`
	Latest     string    `json:"latest_commit,omitempty"`
	Title      string    `json:"latest_title,omitempty"`
	Date       string    `json:"latest_date,omitempty"`
	CompareURL string    `json:"compare_url,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// updateChecker keeps one result. A request never waits on the network: it gets what is known and
// starts one background read if that is stale.
type updateChecker struct {
	mu      sync.Mutex
	status  updateStatus
	running bool
	http    *http.Client
}

func (u *updateChecker) current(cfg config.Updates, running version.Info) updateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	stale := !u.status.Checked || time.Since(u.status.CheckedAt) > cfg.Interval()
	if stale && !u.running {
		u.running = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			got := u.read(ctx, cfg, running)
			got.Checked, got.CheckedAt = true, time.Now().UTC()
			u.mu.Lock()
			u.status, u.running = got, false
			u.mu.Unlock()
		}()
	}
	return u.status
}

func (u *updateChecker) read(ctx context.Context, cfg config.Updates, running version.Info) updateStatus {
	if running.Commit == "" {
		return updateStatus{Error: "This build has no commit stamp, so it cannot be compared."}
	}
	api := strings.TrimRight(cfg.APIURL, "/") + "/repos/" + cfg.Repository + "/compare/" + url.PathEscape(running.Commit) + "..." + url.PathEscape(cfg.Branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return updateStatus{Error: "The update source is misconfigured."}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := u.http
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		log.Printf("update check: %v", err)
		return updateStatus{Error: "The update source could not be reached."}
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return updateStatus{Error: fmt.Sprintf("This build is not on %s of %s.", cfg.Branch, cfg.Repository)}
	}
	if res.StatusCode != http.StatusOK {
		log.Printf("update check: %s answered %d", cfg.APIURL, res.StatusCode)
		return updateStatus{Error: "The update source did not answer usefully."}
	}
	var cmp struct {
		Status  string `json:"status"`
		AheadBy int    `json:"ahead_by"`
		HTMLURL string `json:"html_url"`
		Commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message   string `json:"message"`
				Committer struct {
					Date string `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		} `json:"commits"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&cmp); err != nil {
		return updateStatus{Error: "The update source answered in an unexpected form."}
	}
	out := updateStatus{Behind: cmp.AheadBy, CompareURL: cmp.HTMLURL}
	out.Available = cmp.AheadBy > 0 && (cmp.Status == "ahead" || cmp.Status == "diverged")
	if n := len(cmp.Commits); n > 0 {
		last := cmp.Commits[n-1]
		out.Latest = last.SHA
		out.Title, _, _ = strings.Cut(last.Commit.Message, "\n")
		out.Date = last.Commit.Committer.Date
	}
	return out
}

// updateCommand is what to run here. The server knows how it was deployed; the panel just shows it.
func updateCommand(cfg config.Updates, getenv func(string) string) string {
	if cfg.Command != "" {
		return cfg.Command
	}
	switch getenv("SHIPPIN_DEPLOY_METHOD") {
	case "compose":
		return "git pull --ff-only && docker compose up -d --build"
	case "installer":
		if dir, site := getenv("SHIPPIN_SOURCE_DIR"), getenv("SHIPPIN_SITE_FILE"); dir != "" && site != "" {
			return "cd " + dir + " && git pull --ff-only && sudo ./install/panel/install.sh --site " + site
		}
	}
	return ""
}

// versionInfo is this panel's build and, for operators, whether a newer one exists.
func (a *API) versionInfo(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	if c.op == "" {
		auth.Problem(w, http.StatusForbidden, "Build information needs an operator role.")
		return
	}
	running := version.Current()
	out := map[string]any{"running": running, "short": running.Short(), "command": updateCommand(a.cfg.Updates, os.Getenv)}
	if a.cfg.Updates.Enabled {
		out["update"] = a.updates.current(a.cfg.Updates, running)
		out["source"] = a.cfg.Updates.Repository + "@" + a.cfg.Updates.Branch
	}
	writeJSON(w, out)
}
