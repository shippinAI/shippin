package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// GitOp is one allowlisted git operation in a devspace (R-J1a): "clone" a GitHub repo into a new
// folder, or "publish" an existing folder (init if needed, first commit, origin, push). Every
// command is a fixed argv run as the image user; the short-lived token goes through the image's
// credential helper (pushGitToken), never onto a command line.
type GitOp struct {
	Op     string    `json:"op"`
	Path   string    `json:"path"`   // absolute, under /home/dev
	Repo   string    `json:"repo"`   // owner/name on github.com
	Branch string    `json:"branch"` // clone: branch to check out; publish: branch to push to
	Token  *GitToken `json:"git_token"`
}

var (
	gitPathRe   = regexp.MustCompile(`^/home/dev(/[A-Za-z0-9][A-Za-z0-9._-]{0,99}){1,6}$`)
	gitRepoRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	gitBranchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
)

// githubBase is the clone/push host (a test seam; always github.com in production).
var githubBase = "https://github.com/"

// publishExcludes keep dependency trees and env files out of the first commit when the folder has
// no .gitignore saying otherwise.
var publishExcludes = []string{":!node_modules", ":!.venv", ":!vendor", ":!.cache", ":!dist", ":!.env", ":!.env.*", ":!*.pem", ":!*.key"}

func (g GitOp) check() error {
	if (g.Op != "clone" && g.Op != "publish") || !gitPathRe.MatchString(g.Path) || path.Clean(g.Path) != g.Path ||
		strings.Contains(g.Path, "..") || !gitRepoRe.MatchString(g.Repo) || strings.Contains(g.Repo, "..") ||
		!gitBranchRe.MatchString(g.Branch) || strings.Contains(g.Branch, "..") {
		return fmt.Errorf("invalid git operation")
	}
	return nil
}

// gitStep is one argv; a probe's failure is an answer ("no"), not an error.
type gitStep struct {
	argv  []string
	probe bool
}

// gitPlan is the fixed command list for an op. Probes decide the publish branches (see run).
func gitPlan(g GitOp) []gitStep {
	url := githubBase + g.Repo + ".git"
	G := func(a ...string) []string { return append([]string{"git", "-C", g.Path}, a...) }
	id := []string{"-c", "user.name=Shippin", "-c", "user.email=noreply@shippin.cloud"}
	if g.Op == "clone" {
		return []gitStep{{argv: []string{"test", "!", "-e", g.Path}}, {argv: []string{"git", "clone", "--branch", g.Branch, "--", url, g.Path}}}
	}
	return []gitStep{
		{argv: []string{"test", "-d", g.Path}},
		{argv: []string{"test", "-e", g.Path + "/.git"}, probe: true},                                  // 1: repo?
		{argv: G("init", "-q", "-b", g.Branch)},                                                        // 2: only if not a repo
		{argv: G("remote", "get-url", "origin"), probe: true},                                          // 3: has origin?
		{argv: G("remote", "add", "origin", url)},                                                      // 4: only if no origin
		{argv: G("rev-parse", "--verify", "-q", "HEAD"), probe: true},                                  // 5: has a commit?
		{argv: append(G("add", "-A", "--", "."), publishExcludes...)},                                  // 6: only if no commit
		{argv: append(G(id...), "commit", "-q", "--allow-empty", "-m", "Initial commit from Shippin")}, // 7: only if no commit
		// scan, empty-remote check and push happen on the host from an exported bundle (hostPublish)
	}
}

// RunGit validates and runs op in the devspace w. A publish refuses a folder whose origin is already set.
func (s *Server) RunGit(ctx context.Context, t, w string, g GitOp) error {
	if err := g.check(); err != nil {
		return err
	}
	at, err := s.placeOf(ctx, t, w)
	if err != nil {
		return err
	}
	inst := Instance(w)
	if g.Op == "publish" {
		// the push token never enters the devspace: the host pushes (hostPublish)
		if g.Token == nil || !ghsRe.MatchString(g.Token.Token) || !g.Token.ExpiresAt.After(time.Now()) {
			return fmt.Errorf("refused: publish needs a valid push token")
		}
	} else if err := s.pushGitToken(ctx, at, inst, g.Token); err != nil {
		return err
	}
	uid := fmt.Sprint(s.cfg.Git.UID)
	run := func(argv []string) error {
		args := append([]string{"exec", inst, "--user", uid, "--group", uid, "--env", "HOME=/home/dev", "--env", "GIT_TERMINAL_PROMPT=0", "--"}, argv...)
		_, err := at.do(ctx, args...)
		return err
	}
	err = execPlan(gitPlan(g), run)
	if err == nil && g.Op == "publish" {
		err = s.hostPublish(ctx, at, inst, g)
	}
	if g.Token != nil && g.Op == "publish" && s.cfg.Git.TokenPath != "" {
		// audit b1r3 5(i): the push token never outlives the publish
		if _, rmErr := at.do(ctx, "exec", inst, "--", "rm", "-f", s.cfg.Git.TokenPath); rmErr != nil && err == nil {
			err = fmt.Errorf("git token not removed")
		}
	}
	return err
}

// execPlan applies the publish branching: skip init when already a repo, refuse an existing origin,
// skip the first commit when HEAD exists. Clone plans are linear.
func execPlan(plan []gitStep, run func([]string) error) error {
	skip := map[int]bool{}
	for i, st := range plan {
		if skip[i] {
			continue
		}
		err := run(st.argv)
		switch {
		case !st.probe && err != nil:
			return fmt.Errorf("git step %d failed", i)
		case st.probe && i == 1 && err == nil:
			skip[2] = true
		case st.probe && i == 3 && err == nil:
			return fmt.Errorf("folder already has a git remote")
		case st.probe && i == 5 && err == nil:
			skip[6], skip[7] = true, true
		}
	}
	return nil
}

func (s *Server) gitOp(w http.ResponseWriter, r *http.Request) {
	var g GitOp
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&g) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	if err := s.RunGit(r.Context(), r.PathValue("t"), r.PathValue("w"), g); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}
