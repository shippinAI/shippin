package hostd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func aiFixtureReplies() []fakeReply {
	return []fakeReply{
		{match: "/volumes/custom/ai-", out: `{"config":{"user.shippin.account":"` + acct + `"},"used_by":["/1.0/instances/u-` + acct + `?project=t-` + tid + `"]}`},
		{match: "/1.0/instances/u-" + acct + "?", out: `{"config":{"user.shippin.utility":"` + acct + `","user.shippin.tenant":"` + tid + `"},"expanded_devices":{"ai-home":{"type":"disk","pool":"p-` + tid + `","source":"ai-` + acct + `","path":"/home/dev/.ai"}}}`},
	}
}

func TestAIHomeRejectsMetadataMismatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		index    int
		from, to string
	}{
		{"volume owner", 0, acct, "019a0000-0000-7000-8000-0000000000b2"},
		{"shared volume", 0, "/instances/u-", "/instances/w-"},
		{"other project", 0, "?project=t-", "?project=other-"},
		{"instance owner", 1, `"user.shippin.utility":"` + acct + `"`, `"user.shippin.utility":"other"`},
		{"wrong source", 1, `"source":"ai-`, `"source":"wrong-`},
		{"wrong pool", 1, `"pool":"p-`, `"pool":"other-`},
		{"wrong path", 1, "/home/dev/.ai", "/tmp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replies := aiFixtureReplies()
			replies[tc.index].out = strings.ReplaceAll(replies[tc.index].out, tc.from, tc.to)
			f := useFake(t, replies...)
			if err := aiServer().mountAIHome(context.Background(), place{t: tid}, UtilityInstance(acct), acct); err == nil {
				t.Fatal("accepted unsafe home")
			}
			if f.find("config set") != nil || f.find("storage volume attach") != nil {
				t.Fatal("mutated unsafe attachment")
			}
		})
	}
}

func TestAIHomeAttachConflictNotRelabeled(t *testing.T) {
	r := aiFixtureReplies()
	r[1].out = `{"config":{"user.shippin.utility":"` + acct + `","user.shippin.tenant":"` + tid + `"},"expanded_devices":{}}`
	f := useFake(t, append(r, fakeReply{match: "storage volume attach", err: errors.New("already exists")})...)
	if err := aiServer().mountAIHome(context.Background(), place{t: tid}, UtilityInstance(acct), acct); err == nil {
		t.Fatal("conflict accepted")
	}
	if f.find("config set") != nil {
		t.Fatal("conflicting volume relabeled")
	}
}

func TestWorkspaceCredentialRequestsHaveNoSideEffects(t *testing.T) {
	s := aiServer()
	f := useFake(t)
	if _, err := s.CreateWorkspace(context.Background(), tid, "w", WorkspaceReq{AIHome: acct}); err == nil {
		t.Fatal("create allowed")
	}
	if err := s.AttachAIHome(context.Background(), tid, "w", acct); err == nil {
		t.Fatal("attach allowed")
	}
	if len(f.calls) != 0 {
		t.Fatal("denial must precede Incus calls")
	}
}

func TestAIStatusFailsClosed(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	for name, report := range map[string]string{
		"old schema":       `{"version":1,"checked_at":"` + now + `"}`,
		"stale":            `{"version":2,"checked_at":"2026-01-01T00:00:00Z"}`,
		"future":           `{"version":2,"checked_at":"2100-01-01T00:00:00Z"}`,
		"duplicate":        `{"version":2,"checked_at":"` + now + `","providers":[{"provider":"codex"},{"provider":"codex"}]}`,
		"unknown positive": `{"version":2,"checked_at":"` + now + `","providers":[{"provider":"codex","signed_in":true}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			useFake(t, fakeReply{match: "head -c", out: report})
			if _, err := aiServer().readAIStatus(context.Background(), place{t: tid}, "u-x"); err == nil {
				t.Fatal("untrusted status accepted")
			}
		})
	}
	f := useFake(t, fakeReply{match: "systemctl start", err: errors.New("probe failed")})
	if _, err := aiServer().readAIStatus(context.Background(), place{t: tid}, "u-x"); err == nil {
		t.Fatal("refresh failure ignored")
	}
	if f.find("head -c") != nil {
		t.Fatal("stale report read after failed refresh")
	}
}
