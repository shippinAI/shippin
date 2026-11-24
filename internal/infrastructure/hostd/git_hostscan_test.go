package hostd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// publishRig: a devspace folder, a local bare "github" and a stub gitleaks pinned by sha256.
// streamCmd runs the devspace side locally (argv after "--"); hostExec records every host argv.
type publishRig struct {
	s       *Server
	remote  string
	tok     *GitToken
	argv    [][]string
	scanned []string
}

func newPublishRig(t *testing.T, gitleaks string) *publishRig {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	githubBase = tmp + "/remote/"
	t.Cleanup(func() { githubBase = "https://github.com/" })
	r := &publishRig{remote: tmp + "/remote/o/site.git", tok: &GitToken{Token: "ghs_" + strings.Repeat("A", 36), ExpiresAt: time.Now().Add(time.Hour)}}
	_ = exec.Command("git", "init", "-q", "--bare", r.remote).Run()
	stub := filepath.Join(tmp, "gitleaks")
	_ = os.WriteFile(stub, []byte("#!/bin/sh\n"+gitleaks+"\n"), 0o755)
	b, _ := os.ReadFile(stub)
	sum := sha256.Sum256(b)
	r.s = &Server{cfg: &Config{Git: GitConfig{UID: 1000, Gitleaks: GitleaksPin{SHA256: hex.EncodeToString(sum[:]), HostPath: stub}}}}
	oldS, oldH := streamCmd, hostExec
	streamCmd = func(ctx context.Context, _ place, args []string) *exec.Cmd {
		i := slices.Index(args, "--")
		return exec.CommandContext(ctx, args[i+1], args[i+2:]...)
	}
	hostExec = func(ctx context.Context, env []string, out io.Writer, bin string, args ...string) error {
		r.argv = append(r.argv, append([]string{bin}, args...))
		return oldH(ctx, env, out, bin, args...)
	}
	t.Cleanup(func() { streamCmd, hostExec = oldS, oldH })
	oldScratch := scratchFor
	scratchFor = func(context.Context, string, string) (string, error) {
		return os.MkdirTemp(t.TempDir(), "shippin-")
	}
	t.Cleanup(func() { scratchFor = oldScratch })
	return r
}

func (r *publishRig) folder(t *testing.T, files map[string]string) GitOp {
	d := filepath.Join(t.TempDir(), "site")
	for f, body := range files {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(d, f)), 0o755)
		_ = os.WriteFile(filepath.Join(d, f), []byte(body), 0o644)
	}
	g := GitOp{Op: "publish", Path: d, Repo: "o/site", Branch: "main", Token: r.tok}
	run := func(a []string) error {
		c := exec.Command(a[0], a[1:]...)
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		return c.Run()
	}
	if err := execPlan(gitPlan(g), run); err != nil {
		t.Fatal(err)
	}
	return g
}

func (r *publishRig) heads() string {
	out, _ := exec.Command("git", "--git-dir", r.remote, "for-each-ref").Output()
	return string(out)
}

func TestHostPublishCleanPushesFromBundle(t *testing.T) {
	r := newPublishRig(t, `exit 0`)
	g := r.folder(t, map[string]string{"index.html": "hi", "draft.txt": "untracked later"})
	if err := r.s.hostPublish(context.Background(), place{}, "w", g); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.heads(), "refs/heads/main") {
		t.Fatal("not pushed")
	}
	for _, a := range r.argv {
		if strings.Contains(strings.Join(a, " "), r.tok.Token) {
			t.Fatalf("token on host argv: %v", a)
		}
	}
	if !slices.ContainsFunc(r.argv, func(a []string) bool { return a[0] == r.s.cfg.Git.Gitleaks.HostPath && a[1] == "dir" }) {
		t.Fatal("working tree not scanned on the host")
	}
	if err := r.s.hostPublish(context.Background(), place{}, "w", g); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("second publish into a non-empty repo must refuse: %v", err)
	}
}

func TestHostPublishFindingsRefuse(t *testing.T) {
	for mode, want := range map[string]string{`[ "$1" = git ] && exit 1; exit 0`: "history", `[ "$1" = dir ] && exit 1; exit 0`: "working tree"} {
		r := newPublishRig(t, mode)
		g := r.folder(t, map[string]string{"index.html": "hi"})
		err := r.s.hostPublish(context.Background(), place{}, "w", g)
		if err == nil || !strings.Contains(err.Error(), want) || r.heads() != "" {
			t.Fatalf("%s finding: %v (remote %q)", want, err, r.heads())
		}
	}
	r := newPublishRig(t, `exit 0`)
	g := r.folder(t, map[string]string{"index.html": "hi"})
	r.s.cfg.Git.Gitleaks.SHA256 = strings.Repeat("0", 64)
	if err := r.s.hostPublish(context.Background(), place{}, "w", g); err == nil || r.heads() != "" {
		t.Fatalf("a binary off the pin must refuse: %v", err)
	}
}
