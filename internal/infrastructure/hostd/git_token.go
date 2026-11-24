package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// GitConfig (M3) is where a workspace's short-lived GitHub installation token lives: a tmpfs file
// owned by the image user, read by the image's git credential helper. Empty token_path disables it.
type GitConfig struct {
	TokenPath string      `json:"token_path"`
	UID       int         `json:"uid"`
	Gitleaks  GitleaksPin `json:"gitleaks"`
}

// GitToken is a 1-hour installation token narrowed to the workspace's repository.
type GitToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

var (
	ghsRe = regexp.MustCompile(`^ghs_[A-Za-z0-9_]{20,255}$`)
	// githubRepoRe: github:<owner>/<repo>[@<branch>], the form only the console's import job sends.
	githubRepoRe = regexp.MustCompile(`^github:[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9][A-Za-z0-9._-]{0,99}(@[A-Za-z0-9][A-Za-z0-9._/-]{0,199})?$`)
)

// ValidGitHubRepo checks an import source (no "..", no option-like branch).
func ValidGitHubRepo(repo string) error {
	if !githubRepoRe.MatchString(repo) || strings.Contains(repo, "..") {
		return fmt.Errorf("invalid github repository")
	}
	return nil
}

func (g GitConfig) check() error {
	if err := g.Gitleaks.check(); err != nil {
		return err
	}
	if g.TokenPath == "" {
		return nil
	}
	if !strings.HasPrefix(g.TokenPath, "/run/") || strings.Contains(g.TokenPath, "..") || g.UID < 1 {
		return fmt.Errorf("git.token_path must be under /run and git.uid set")
	}
	return nil
}

// pushGitToken writes the token into inst as the image user, mode 0600, replacing it atomically.
func (s *Server) pushGitToken(ctx context.Context, at place, inst string, t *GitToken) error {
	g := s.cfg.Git
	if t == nil {
		return nil
	}
	if g.TokenPath == "" {
		return fmt.Errorf("git tokens are not configured")
	}
	if !ghsRe.MatchString(t.Token) || !t.ExpiresAt.After(time.Now()) {
		return fmt.Errorf("invalid git token")
	}
	uid := fmt.Sprint(g.UID)
	dir := g.TokenPath[:strings.LastIndex(g.TokenPath, "/")]
	if _, err := at.do(ctx, "exec", inst, "--", "install", "-d", "-m", "0700", "-o", uid, "-g", uid, dir); err != nil {
		return err
	}
	_, err := at.inc(ctx, t.Token+"\n", "exec", inst, "--user", uid, "--group", uid, "--env", "F="+g.TokenPath, "--",
		"sh", "-c", `umask 077 && cat > "$F.tmp" && mv -f "$F.tmp" "$F"`)
	if err != nil {
		return fmt.Errorf("git token not installed")
	}
	return nil
}

// PutGitToken refreshes the token of a running workspace.
func (s *Server) PutGitToken(ctx context.Context, t, w string, tok GitToken) error {
	at, err := s.placeOf(ctx, t, w)
	if err != nil {
		return err
	}
	return s.pushGitToken(ctx, at, Instance(w), &tok)
}

func (s *Server) putGitToken(w http.ResponseWriter, r *http.Request) {
	var tok GitToken
	if json.NewDecoder(r.Body).Decode(&tok) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	if err := s.PutGitToken(r.Context(), r.PathValue("t"), r.PathValue("w"), tok); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}
