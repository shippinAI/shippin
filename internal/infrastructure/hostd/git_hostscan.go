package hostd

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func basicAuth(u, p string) string { return base64.StdEncoding.EncodeToString([]byte(u + ":" + p)) }

// exportWorkspace streams a git bundle (HEAD + every ref) and a tar of the working tree (untracked
// files included, .git excluded) out of the devspace into dir on the host. The tenant has root in
// the devspace, so these files are treated as hostile input: they are only ever scanned, and the
// push is made on the host from this very bundle, so what is pushed is exactly what was scanned.
func (s *Server) exportWorkspace(ctx context.Context, at place, inst string, g GitOp, dir string) error {
	return s.exportFolder(ctx, at, inst, g.Path, dir, true)
}

// exportFolder is exportWorkspace for any folder; withBundle=false exports only the working tree.
func (s *Server) exportFolder(ctx context.Context, at place, inst, folder, dir string, withBundle bool) error {
	uid := fmt.Sprint(s.cfg.Git.UID)
	pre := []string{"exec", "--force-noninteractive", inst, "--user", uid, "--group", uid, "--env", "HOME=/home/dev", "--"}
	out := map[string][]string{"tree.tar": {"tar", "-C", folder, "--exclude=./.git", "-cf", "-", "."}}
	if withBundle {
		out["repo.bundle"] = []string{"git", "-C", folder, "bundle", "create", "-", "HEAD", "--all"}
	}
	for name, argv := range out {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		cmd := streamCmd(ctx, at, append(append([]string{}, pre...), argv...))
		cmd.Stdout = f
		err = cmd.Run()
		cancel()
		f.Close()
		if err != nil {
			return fmt.Errorf("refused: could not export %s from the devspace", name)
		}
	}
	return nil
}

// hostScanAndPush (B1 follow-up) runs the pinned gitleaks ON THE HOST over the exported history and
// working tree, fails closed on any error, checks the linked repo is empty and pushes the bundle's
// HEAD from the host. The token stays on the host, in env only.
func (s *Server) hostScanAndPush(ctx context.Context, g GitOp, dir string) error {
	if err := s.verifyHostGitleaks(); err != nil {
		return err
	}
	leaks := s.cfg.Git.Gitleaks.HostPath
	bare, tree := filepath.Join(dir, "repo.git"), filepath.Join(dir, "tree")
	env, url := pushEnv(dir, g.Token), githubBase+g.Repo+".git"
	if err := os.Mkdir(tree, 0o700); err != nil {
		return err
	}
	steps := []struct {
		bin  string
		args []string
		fail string
	}{
		{"git", []string{"clone", "--bare", "--quiet", "--", filepath.Join(dir, "repo.bundle"), bare}, "the exported history is not a valid git bundle"},
		{"tar", []string{"-xf", filepath.Join(dir, "tree.tar"), "-C", tree, "--no-same-owner", "--no-same-permissions"}, "the exported working tree is not a valid archive"},
		{leaks, []string{"git", "--no-banner", "--redact", "--exit-code", "1", bare}, "secret scan (gitleaks, history) found a secret or could not run"},
		{leaks, []string{"dir", "--no-banner", "--redact", "--exit-code", "1", tree}, "secret scan (gitleaks, working tree) found a secret or could not run"},
	}
	for _, st := range steps {
		if err := s.hostRun(ctx, env, nil, st.bin, st.args...); err != nil {
			return fmt.Errorf("refused: %s", st.fail)
		}
	}
	var refs bytes.Buffer
	if err := s.hostRun(ctx, env, &refs, "git", "ls-remote", "--", url); err != nil || refs.Len() > 0 {
		return fmt.Errorf("refused: the linked repository is not empty")
	}
	if err := s.hostRun(ctx, env, nil, "git", "-C", bare, "push", "--quiet", url, "HEAD:refs/heads/"+g.Branch); err != nil {
		return fmt.Errorf("push failed")
	}
	return nil
}

// hostPublish exports, scans and pushes in a private scratch dir on the org dataset.
func (s *Server) hostPublish(ctx context.Context, at place, inst string, g GitOp) error {
	dir, err := s.scratch(ctx, at.t)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := s.exportWorkspace(ctx, at, inst, g, dir); err != nil {
		return err
	}
	return s.hostScanAndPush(ctx, g, dir)
}

// hostScanFolder (B2) is the one-click Publish scan: the same host-side path as GitHub publish
// (export from the devspace, pinned gitleaks on the HOST, fail closed). The folder's git history is
// scanned too when it is a repository. Nothing is pushed or snapshotted here.
func (s *Server) hostScanFolder(ctx context.Context, at place, inst, folder string) error {
	if !gitPathRe.MatchString(folder) || filepath.Clean(folder) != folder || strings.Contains(folder, "..") {
		return fmt.Errorf("invalid app folder")
	}
	return s.scanFolder(ctx, at, inst, folder)
}

func (s *Server) scanFolder(ctx context.Context, at place, inst, folder string) error {
	if err := s.verifyHostGitleaks(); err != nil {
		return err
	}
	dir, err := s.scratch(ctx, at.t)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	uid := fmt.Sprint(s.cfg.Git.UID)
	if _, err := at.do(ctx, "exec", inst, "--user", uid, "--group", uid, "--", "test", "-d", folder); err != nil {
		return fmt.Errorf("refused: app folder not found in the devspace")
	}
	_, gerr := at.do(ctx, "exec", inst, "--user", uid, "--group", uid, "--", "git", "-C", folder, "rev-parse", "--verify", "-q", "HEAD")
	isRepo := gerr == nil
	if err := s.exportFolder(ctx, at, inst, folder, dir, isRepo); err != nil {
		return err
	}
	leaks, tree, bare := s.cfg.Git.Gitleaks.HostPath, filepath.Join(dir, "tree"), filepath.Join(dir, "repo.git")
	env := pushEnv(dir, nil)
	if err := os.Mkdir(tree, 0o700); err != nil {
		return err
	}
	if err := s.hostRun(ctx, env, nil, "tar", "-xf", filepath.Join(dir, "tree.tar"), "-C", tree, "--no-same-owner", "--no-same-permissions"); err != nil {
		return fmt.Errorf("refused: the exported app folder is not a valid archive")
	}
	if isRepo {
		if err := s.hostRun(ctx, env, nil, "git", "clone", "--bare", "--quiet", "--", filepath.Join(dir, "repo.bundle"), bare); err != nil {
			return fmt.Errorf("refused: the exported history is not a valid git bundle")
		}
		if err := s.hostRun(ctx, env, nil, leaks, "git", "--no-banner", "--redact", "--exit-code", "1", bare); err != nil {
			return fmt.Errorf("refused: secret scan (gitleaks, history) found a secret or could not run")
		}
	}
	if err := s.hostRun(ctx, env, nil, leaks, "dir", "--no-banner", "--redact", "--exit-code", "1", tree); err != nil {
		return fmt.Errorf("refused: secret scan (gitleaks, app folder) found a secret or could not run")
	}
	return nil
}
