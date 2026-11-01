package hostd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// hostExec runs a fixed host binary for the publish scan: only git, tar and the pinned gitleaks.
// env carries the push credential (never argv). It is the test seam for the host side.
var hostExec = func(ctx context.Context, env []string, stdout io.Writer, bin string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed", bin, args[0]) // stderr may carry a redacted finding; dropped
	}
	return nil
}

func (s *Server) hostRun(ctx context.Context, env []string, stdout io.Writer, bin string, args ...string) error {
	if bin != "git" && bin != "tar" && bin != s.cfg.Git.Gitleaks.HostPath {
		return fmt.Errorf("binary not allowed")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	return hostExec(ctx, env, stdout, bin, args...)
}

// verifyHostGitleaks proves the host binary matches the pin right before each scan.
func (s *Server) verifyHostGitleaks() error {
	p := s.cfg.Git.Gitleaks
	if p.HostPath == "" {
		return fmt.Errorf("refused: no pinned gitleaks configured (git.gitleaks)")
	}
	b, err := readHostFile(p.HostPath)
	if err != nil {
		return fmt.Errorf("refused: pinned gitleaks missing on the host")
	}
	if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != p.SHA256 {
		return fmt.Errorf("refused: host gitleaks sha256 does not match the pin")
	}
	return nil
}

// pushEnv authenticates host git to github.com through env-only config (GIT_CONFIG_*), with no
// user or system gitconfig, no prompts and HOME in the scratch dir.
func pushEnv(home string, tok *GitToken) []string {
	env := []string{"HOME=" + home, "PATH=/usr/bin:/bin", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	if tok == nil {
		return env
	}
	basic := basicAuth("x-access-token", tok.Token)
	return append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http."+strings.TrimSuffix(githubBase, "/")+"/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+basic)
}
