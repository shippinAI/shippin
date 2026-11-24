package hostd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitOpCheck(t *testing.T) {
	ok := GitOp{Op: "publish", Path: "/home/dev/box-work/projects/site", Repo: "eonsofstupid/site", Branch: "main"}
	if err := ok.check(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []GitOp{
		{Op: "rm", Path: ok.Path, Repo: ok.Repo, Branch: "main"},
		{Op: "publish", Path: "/etc/passwd", Repo: ok.Repo, Branch: "main"},
		{Op: "publish", Path: "/home/dev/../root", Repo: ok.Repo, Branch: "main"},
		{Op: "publish", Path: "/home/dev/-rf", Repo: ok.Repo, Branch: "main"},
		{Op: "publish", Path: "/home/dev", Repo: ok.Repo, Branch: "main"},
		{Op: "publish", Path: ok.Path, Repo: "a/b;rm -rf /", Branch: "main"},
		{Op: "publish", Path: ok.Path, Repo: "../x", Branch: "main"},
		{Op: "clone", Path: ok.Path, Repo: ok.Repo, Branch: "--upload-pack=x"},
		{Op: "clone", Path: ok.Path, Repo: ok.Repo, Branch: "a..b"},
	} {
		if bad.check() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestExecPlanBranches(t *testing.T) {
	g := GitOp{Op: "publish", Path: "/home/dev/site", Repo: "o/site", Branch: "main"}
	var ran []string
	// already a repo with commits, no origin: no init, no commit, remote add + push
	err := execPlan(gitPlan(g), func(a []string) error {
		ran = append(ran, strings.Join(a, " "))
		if strings.Contains(ran[len(ran)-1], "remote get-url") {
			return os.ErrNotExist
		}
		return nil
	})
	got := strings.Join(ran, "\n")
	if err != nil || strings.Contains(got, " init ") || strings.Contains(got, " commit ") || !strings.Contains(got, "remote add origin https://github.com/o/site.git") {
		t.Fatalf("%v\n%s", err, got)
	}
	// origin already set: refused before any write
	ran = nil
	err = execPlan(gitPlan(g), func(a []string) error { ran = append(ran, strings.Join(a, " ")); return nil })
	if err == nil || strings.Contains(strings.Join(ran, "\n"), "push") {
		t.Fatalf("existing origin must be refused: %v %v", err, ran)
	}
}

// Real git: publish a plain folder to a local bare repo (stand-in for github.com) and clone it back.
func TestPublishAndCloneOnDisk(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	githubBase = tmp + "/remote/"
	t.Cleanup(func() { githubBase = "https://github.com/" })
	if b, err := exec.Command("git", "init", "-q", "--bare", tmp+"/remote/o/site.git").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	mk := func(name string) string {
		d := filepath.Join(tmp, name)
		for f, body := range map[string]string{"index.html": "hi", ".env": "SECRET=1", "node_modules/x/i.js": "x", "key.pem": "k"} {
			_ = os.MkdirAll(filepath.Dir(filepath.Join(d, f)), 0o755)
			_ = os.WriteFile(filepath.Join(d, f), []byte(body), 0o644)
		}
		return d
	}
	src := mk("site")
	run := func(a []string) error {
		c := exec.Command(a[0], a[1:]...)
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		return c.Run()
	}
	if err := execPlan(gitPlan(GitOp{Op: "publish", Path: src, Repo: "o/site", Branch: "main"}), run); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"git", "-C", src, "push", "-q", "origin", "HEAD:refs/heads/main"}); err != nil {
		t.Fatal(err) // the real push is the host's (git_hostscan_test.go); here: prep committed the right files
	}
	out := filepath.Join(tmp, "copy")
	if err := execPlan(gitPlan(GitOp{Op: "clone", Path: out, Repo: "o/site", Branch: "main"}), run); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "index.html")); string(b) != "hi" {
		t.Fatal("clone is missing index.html")
	}
	for _, f := range []string{".env", "node_modules", "key.pem"} {
		if _, err := os.Stat(filepath.Join(out, f)); err == nil {
			t.Errorf("%s was pushed", f)
		}
	}
	if err := execPlan(gitPlan(GitOp{Op: "clone", Path: out, Repo: "o/site", Branch: "main"}), run); err == nil {
		t.Fatal("clone over an existing folder must fail")
	}
}
