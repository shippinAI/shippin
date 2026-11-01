package hostd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDiscover(t *testing.T) {
	out := strings.Join([]string{
		"git\t./code/box-work\thttps://operator:ghp_SECRET@github.com/eonsofstupid/box-work.git",
		"manifest\t./code/box-work\tpackage.json",
		"manifest\t./code/box-work/web\tpackage.json", // inside the repo: dropped
		"manifest\t./notes-site\tgo.mod",
		"manifest\t./notes-site\tpackage.json",
		"git\t.\t",            // the home itself: dropped
		"manifest\t../etc\tx", // escape attempt: dropped
		"garbage",
	}, "\n")
	got := parseDiscover(out)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	bw, ns := got[0], got[1]
	if bw.Path != "/home/dev/code/box-work" || !bw.Git || bw.Remote != "https://github.com/eonsofstupid/box-work.git" || bw.Name != "box-work" {
		t.Errorf("box-work = %+v", bw)
	}
	if strings.Contains(bw.Remote, "SECRET") {
		t.Fatal("credential leaked from remote URL")
	}
	if ns.Git || strings.Join(ns.Manifests, ",") != "go.mod,package.json" {
		t.Errorf("notes-site = %+v", ns)
	}
}

func TestScrubRemote(t *testing.T) {
	for in, want := range map[string]string{
		"https://u:p@github.com/a/b.git": "https://github.com/a/b.git",
		"git@github.com:a/b.git":         "git@github.com:a/b.git",
		"https://github.com/a/b@c":       "https://github.com/a/b@c",
		"":                               "",
	} {
		if got := scrubRemote(in); got != want {
			t.Errorf("scrubRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

// The real script, run against a temp home: proves the find/prune/git-config pipeline (needs git, sh).
func TestDiscoverScriptOnDisk(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	repo := filepath.Join(home, "code", "box-work")
	for _, d := range []string{repo, filepath.Join(home, "notes-site", "node_modules", "x"), filepath.Join(home, ".cache", "y")} {
		_ = os.MkdirAll(d, 0o755)
	}
	run := func(dir string, args ...string) {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, b)
		}
	}
	run(repo, "git", "init", "-q")
	run(repo, "git", "remote", "add", "origin", "https://github.com/eonsofstupid/box-work.git")
	for _, f := range []string{filepath.Join(home, "notes-site", "package.json"), filepath.Join(home, "notes-site", "node_modules", "x", "package.json"), filepath.Join(home, ".cache", "y", "go.mod")} {
		_ = os.WriteFile(f, []byte("{}"), 0o644)
	}
	script := strings.Replace(discoverScript, "cd /home/dev", "cd "+home, 1)
	b, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := parseDiscover(string(b))
	if len(got) != 2 || got[0].Name != "box-work" || got[0].Remote != "https://github.com/eonsofstupid/box-work.git" || got[1].Name != "notes-site" {
		t.Fatalf("got %+v from %q", got, b)
	}
}

// Nested repos under a non-repo folder are found (box-work/projects/*), a repo's subfolders are not
// scanned, skipped dirs are skipped, and nothing deeper than 6 levels is reported.
func TestDiscoverScriptNested(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	mk := func(rel, file string) {
		d := filepath.Join(home, rel)
		_ = os.MkdirAll(d, 0o755)
		if file != "" {
			_ = os.WriteFile(filepath.Join(d, file), []byte("{}"), 0o644)
		}
	}
	for _, r := range []string{"box-work/projects/shippin-console", "box-work/projects/rrflow", "box-work/gold-homebase"} {
		mk(r, "")
		if b, err := exec.Command("git", "init", "-q", filepath.Join(home, r)).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	mk("box-work/projects/shippin-console", "go.mod")
	mk("box-work/projects/shippin-console/web", "package.json") // inside a repo: not scanned
	mk("box-work/projects/rrflow/sub/inner", "package.json")    // inside a repo: not scanned
	mk("box-work/vendor/lib", "go.mod")                         // skipped dir
	mk("box-work/.cache/x", "package.json")                     // skipped dir
	mk("box-work/apps/site/node_modules/dep", "package.json")   // skipped dir
	mk("box-work/apps/site", "package.json")                    // plain folder, depth 3
	mk("a/b/c/d/e/six", "package.json")                         // dir depth 6, file depth 7: too deep
	mk("a/b/c/d/five", "Cargo.toml")                            // file depth 6: found
	script := strings.Replace(discoverScript, "cd /home/dev", "cd "+home, 1)
	b, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range parseDiscover(string(b)) {
		names = append(names, strings.TrimPrefix(f.Path, "/home/dev/"))
	}
	want := "a/b/c/d/five,box-work/apps/site,box-work/gold-homebase,box-work/projects/rrflow,box-work/projects/shippin-console"
	if got := strings.Join(names, ","); got != want {
		t.Fatalf("got  %s\nwant %s\nraw %q", got, want, b)
	}
}
