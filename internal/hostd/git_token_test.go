package hostd

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGitHubRepoForm(t *testing.T) {
	for repo, ok := range map[string]bool{
		"github:operator/app": true, "github:operator/app@main": true, "github:org-1/my.repo@feature/x": true,
		"github:operator/../etc": false, "github:operator/app@-rf": false, "github:operator/app@a..b": false,
		"github:operator/app;id": false, "github:/app": false, "https://github.com/operator/app": false,
	} {
		if (ValidGitHubRepo(repo) == nil) != ok {
			t.Errorf("%q ok=%v", repo, !ok)
		}
	}
	s := testServer()
	if err := s.validTemplate(WorkspaceReq{Repo: "github:operator/app@main"}); err != nil {
		t.Fatalf("import form accepted by hostd: %v", err)
	}
	if ValidRepo("github:operator/app", s.cfg.Edge.RepoHosts) == nil {
		t.Fatal("the user-entered repo field must not accept the import form")
	}
}

func TestPushGitToken(t *testing.T) {
	s := testServer()
	s.cfg.Git = GitConfig{TokenPath: "/run/shippin/git/token", UID: 1000}
	f := useFake(t)
	at := place{t: tid}
	good := &GitToken{Token: "ghs_" + strings.Repeat("a", 36), ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.pushGitToken(context.Background(), at, "w-x", good); err != nil {
		t.Fatal(err)
	}
	c := f.find("exec w-x --user 1000 --group 1000", "F=/run/shippin/git/token", "umask 077")
	if c == nil || c[len(c)-1] != good.Token+"\n" {
		t.Fatalf("token goes over stdin as the image user with umask 077: %v", c)
	}
	for _, j := range c[:len(c)-1] {
		if strings.Contains(j, "ghs_") {
			t.Fatal("token must never be on an argv")
		}
	}
	if f.find("install -d -m 0700 -o 1000 -g 1000 /run/shippin/git") == nil {
		t.Fatal("token dir 0700")
	}
	for _, bad := range []*GitToken{{Token: "ghp_" + strings.Repeat("a", 36), ExpiresAt: good.ExpiresAt},
		{Token: "ghs_short", ExpiresAt: good.ExpiresAt}, {Token: good.Token, ExpiresAt: time.Now().Add(-time.Minute)},
		{Token: good.Token + "\nrm -rf /", ExpiresAt: good.ExpiresAt}} {
		if s.pushGitToken(context.Background(), at, "w-x", bad) == nil {
			t.Fatalf("accepted %q", bad.Token)
		}
	}
	if (GitConfig{TokenPath: "/home/dev/t", UID: 1}).check() == nil {
		t.Fatal("token path must be tmpfs (/run)")
	}
}
