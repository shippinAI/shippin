package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// AIStatus is the image's exit-code-only report of which vendor CLIs the user is signed in to.
// It comes from inside a container, so it is size-bounded and decoded into these types only.
type AIStatus struct {
	Version   int                `json:"version"`
	CheckedAt time.Time          `json:"checked_at"`
	Providers []AIProviderStatus `json:"providers"`
}

type AIProviderStatus struct {
	Provider  string `json:"provider"`
	Installed bool   `json:"installed"`
	SignedIn  bool   `json:"signed_in"`
	AuthKnown bool   `json:"auth_known"`
}

// readAIStatus requires a successful refresh and a fresh, versioned report.
func (s *Server) readAIStatus(ctx context.Context, at place, inst string) (*AIStatus, error) {
	a := s.cfg.AI
	if !a.enabled() {
		return nil, fmt.Errorf("ai activation is not configured")
	}
	started := time.Now()
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "start", a.StatusUnit); err != nil {
		return nil, fmt.Errorf("ai status refresh failed")
	}
	out, err := at.do(ctx, "exec", inst, "--", "head", "-c", fmt.Sprint(a.StatusMaxBytes+1), a.StatusPath)
	if err != nil {
		return nil, fmt.Errorf("ai status unavailable")
	}
	if len(out) > a.StatusMaxBytes {
		return nil, fmt.Errorf("ai status too large")
	}
	var st AIStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		return nil, fmt.Errorf("ai status malformed")
	}
	if st.Version != 2 || st.CheckedAt.Before(started.Add(-2*time.Second)) || st.CheckedAt.After(time.Now().Add(5*time.Second)) {
		return nil, fmt.Errorf("ai status stale or unsupported")
	}
	seen := map[string]bool{}
	for _, p := range st.Providers {
		if !provRe.MatchString(p.Provider) || seen[p.Provider] || (p.SignedIn && (!p.Installed || !p.AuthKnown)) {
			return nil, fmt.Errorf("ai provider status malformed")
		}
		seen[p.Provider] = true
	}
	return &st, nil
}

// WorkspaceAIStatus reads the status inside workspace w.
func (s *Server) WorkspaceAIStatus(ctx context.Context, t, w string) (*AIStatus, error) {
	return nil, fmt.Errorf("provider status requires the isolated owner runner")
}

// UtilityAIStatus reads the status inside the account's activation container.
func (s *Server) UtilityAIStatus(ctx context.Context, t, account string) (*AIStatus, error) {
	at, err := s.utilityAt(ctx, t, account)
	if err != nil {
		return nil, err
	}
	if mounted, err := s.verifyAIHome(ctx, at, UtilityInstance(account), account); err != nil || !mounted {
		return nil, fmt.Errorf("runner isolation could not be verified")
	}
	return s.readAIStatus(ctx, at, UtilityInstance(account))
}

// UtilityLogout runs the vendor's own sign-out as the user in their activation container, which
// removes the login from their AI home. Output is discarded; nothing credential-shaped is returned.
func (s *Server) UtilityLogout(ctx context.Context, t, account, provider string) error {
	argv, ok := s.cfg.AI.Logout[provider]
	if !ok {
		return fmt.Errorf("unknown provider")
	}
	at, err := s.utilityAt(ctx, t, account)
	if err != nil {
		return err
	}
	if mounted, err := s.verifyAIHome(ctx, at, UtilityInstance(account), account); err != nil || !mounted {
		return fmt.Errorf("runner isolation could not be verified before sign-out")
	}
	a := s.cfg.AI
	args := append([]string{"exec", UtilityInstance(account), "--user", fmt.Sprint(a.UID), "--group", fmt.Sprint(a.UID),
		"--env", "HOME=/home/" + a.User, "--", "bash", "-l", "-c", `exec "$@"`, "logout"}, argv...)
	if _, err := at.do(ctx, args...); err != nil {
		return fmt.Errorf("sign-out failed")
	}
	return nil
}
