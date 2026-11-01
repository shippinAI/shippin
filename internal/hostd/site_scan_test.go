package hostd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B2: one-click Publish scans the app folder on the host with the pinned gitleaks (same path as
// GitHub publish), history included when the folder is a repo, and fails closed.
func TestSiteScanFolderHostSide(t *testing.T) {
	for _, tc := range []struct {
		name, leaks string
		repo, ok    bool
	}{{"clean repo", "exit 0", true, true}, {"clean plain folder", "exit 0", false, true}, {"finding", "exit 1", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			r := newPublishRig(t, tc.leaks)
			var d string
			if tc.repo {
				d = r.folder(t, map[string]string{"index.html": "hi"}).Path
			} else {
				d = filepath.Join(t.TempDir(), "app")
				_ = os.MkdirAll(d, 0o755)
				_ = os.WriteFile(filepath.Join(d, "index.html"), []byte("hi"), 0o644)
			}
			var gitProbe error
			if !tc.repo {
				gitProbe = errors.New("not a repo")
			}
			useFake(t, fakeReply{match: "rev-parse", err: gitProbe})
			err := r.s.scanFolder(context.Background(), place{}, "w", d)
			if tc.ok != (err == nil) {
				t.Fatalf("ok=%v err=%v", tc.ok, err)
			}
			hasGit := false
			for _, a := range r.argv {
				if a[0] == r.s.cfg.Git.Gitleaks.HostPath && a[1] == "git" {
					hasGit = true
				}
				if a[0] == "git" && a[1] == "push" {
					t.Fatal("a site scan must never push")
				}
			}
			if hasGit != tc.repo {
				t.Fatalf("history scan ran=%v for repo=%v", hasGit, tc.repo)
			}
		})
	}
	s := &Server{cfg: &Config{}}
	for _, bad := range []string{"/etc", "/home/dev/../root", "/home/dev/a/../../x", ""} {
		if err := s.hostScanFolder(context.Background(), place{}, "w", bad); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if err := s.hostScanFolder(context.Background(), place{}, "w", "/home/dev/code"); err == nil || !strings.Contains(err.Error(), "gitleaks") {
		t.Errorf("no pin must refuse: %v", err)
	}
}
