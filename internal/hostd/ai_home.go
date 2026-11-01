package hostd

import (
	"context"
	"fmt"
	"strings"
)

// AIHomeVolume contains vendor credentials for one isolated owner runner.
func AIHomeVolume(account string) string { return "ai-" + account }

// UtilityInstance is a user's activation container.
func UtilityInstance(account string) string { return "u-" + account }

const aiDevice = "ai-home"

// ensureAIHome creates ai-<account> in the place's pool (VM tenants: the inner default project,
// since pj-/pa- projects share its volumes).
func (s *Server) ensureAIHome(ctx context.Context, at place, account string) error {
	if !s.cfg.AI.enabled() {
		return fmt.Errorf("ai activation is not configured")
	}
	if err := ValidUUID(account); err != nil {
		return fmt.Errorf("invalid account id")
	}
	args := []string{"storage", "volume", "create", at.pool(), AIHomeVolume(account),
		fmt.Sprintf("size=%dGiB", s.cfg.AI.HomeGiB), "user.shippin.account=" + account}
	if s.cfg.AI.Shifted {
		args = append(args, "security.shifted=true")
	}
	return ignoreExists(at.do(ctx, args...))
}

// mountAIHome attaches the account's AI home to inst at ai.home_path (before first start, or
// hot-plugged into a running container) and labels the instance with its owner.
func (s *Server) mountAIHome(ctx context.Context, at place, inst, account string) error {
	if account == "" {
		return nil
	}
	if ValidUUID(account) != nil || inst != UtilityInstance(account) {
		return fmt.Errorf("provider credentials require an isolated owner runner")
	}
	aiMountMu.Lock()
	defer aiMountMu.Unlock()
	if err := s.ensureAIHome(ctx, at, account); err != nil {
		return err
	}
	mounted, err := s.verifyAIHome(ctx, at, inst, account)
	if err != nil {
		return err
	}
	if !mounted {
		if _, err = at.do(ctx, "storage", "volume", "attach", at.pool(), AIHomeVolume(account), inst, aiDevice, s.cfg.AI.HomePath); err != nil {
			return err // Never treat an arbitrary already-exists error as success.
		}
		if mounted, err = s.verifyAIHome(ctx, at, inst, account); err != nil || !mounted {
			return fmt.Errorf("AI home attachment could not be verified")
		}
	}
	_, err = at.do(ctx, "config", "set", inst, "user.shippin.ai_home="+account)
	return err
}

// aiHomeOf reads the AI home owner label of inst ("" when none).
func (s *Server) aiHomeOf(ctx context.Context, at place, inst string) string {
	out, err := at.do(ctx, "config", "get", inst, "user.shippin.ai_home")
	if v := strings.TrimSpace(out); err == nil && ValidUUID(v) == nil {
		return v
	}
	return ""
}

// ownAIHome gives the mount point (not its contents) to the image user, mode 0700.
func (s *Server) ownAIHome(ctx context.Context, at place, inst, account string) error {
	if account == "" {
		return nil
	}
	a := s.cfg.AI
	_, err := at.do(ctx, "exec", inst, "--", "install", "-d", "-o", fmt.Sprint(a.UID), "-g", fmt.Sprint(a.UID),
		"-m", "0700", a.HomePath)
	return err
}

// AttachAIHome rejects the legacy shared-workspace credential attachment API.
func (s *Server) AttachAIHome(ctx context.Context, t, w, account string) error {
	return fmt.Errorf("provider credentials require an isolated owner runner")
}
