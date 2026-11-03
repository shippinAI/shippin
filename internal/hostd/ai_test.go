package hostd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"shippin.cloud/console/internal/subs"
	"strings"
	"testing"
	"time"
)

const acct = "019a0000-0000-7000-8000-0000000000a1"

func aiServer() *Server {
	s := testServer()
	s.cfg.AI = AIConfig{HomePath: "/home/dev/.ai", HomeGiB: 5, Shifted: true, User: "dev", UID: 1000,
		StatusPath: "/run/shippin/ai/status.json", StatusUnit: "shippin-ai-status.service", StatusMaxBytes: 512,
		Utility: AIUtility{Image: "shippin/dev-debian13", CPU: 1, Memory: "1GiB", Port: 7681, Iface: "eth0",
			Unit: "shippin-ttyd.service", EnvFile: "/etc/default/shippin-ttyd"},
		Logout: map[string][]string{"claude": {"claude", "auth", "logout"}}}
	return s
}

func TestAIConfigCheck(t *testing.T) {
	s := aiServer()
	if err := s.cfg.AI.check("shippin/"); err != nil {
		t.Fatal(err)
	}
	if (AIConfig{}).check("shippin/") != nil {
		t.Fatal("disabled section must pass")
	}
	for _, mut := range []func(*AIConfig){
		func(a *AIConfig) { a.HomePath = "/root/.ai" },
		func(a *AIConfig) { a.StatusPath = "/home/dev/x" },
		func(a *AIConfig) { a.Utility.Image = "evil/img" },
		func(a *AIConfig) { a.Utility.EnvFile = "/etc/passwd" },
		func(a *AIConfig) { a.Logout = map[string][]string{"Bad!": {"x"}} },
	} {
		a := aiServer().cfg.AI
		mut(&a)
		if a.check("shippin/") == nil {
			t.Fatalf("must fail: %+v", a)
		}
	}
}

func TestMountAIHome(t *testing.T) {
	s := aiServer()
	f := useFake(t, aiFixtureReplies()...)
	at := place{t: tid}
	if err := s.mountAIHome(context.Background(), at, UtilityInstance(acct), acct); err != nil {
		t.Fatal(err)
	}
	if f.find("storage volume create p-"+tid, "ai-"+acct, "size=5GiB", "security.shifted=true") == nil {
		t.Fatal("AI home volume")
	}
	if f.find("storage volume attach") != nil {
		t.Fatal("verified existing device must be reused")
	}
	if err := s.mountAIHome(context.Background(), at, "w-x", acct); err == nil {
		t.Fatal("shared workspace must never receive provider credentials")
	}
	if err := s.mountAIHome(context.Background(), at, "w-x", "../x"); err == nil {
		t.Fatal("account id validated")
	}
}

func TestStartUtilityLegacy(t *testing.T) {
	s := aiServer()
	f := useFake(t, append(aiFixtureReplies(), fakeReply{match: "/1.0/projects/t-", out: `{"config":{}}`},
		fakeReply{match: "-X DELETE", out: "200"}, fakeReply{match: "-X GET", out: "404"},
		fakeReply{match: "info u-", err: errors.New("not found")},
		fakeReply{match: "/state", out: `{"network":{"eth0":{"addresses":[{"family":"inet","address":"10.1.2.3","scope":"global"}]}}}`})...)
	baseRunner := runner
	inventories := 0
	runner = func(ctx context.Context, input, bin string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "/1.0/instances?recursion=1") {
			inventories++
			if inventories == 1 {
				return "[]", nil
			}
			return `[{"name":"u-` + acct + `","config":{"user.shippin.utility":"` + acct + `","user.shippin.tenant":"` + tid + `"}}]`, nil
		}
		return baseRunner(ctx, input, bin, args...)
	}
	ip, err := s.StartUtility(context.Background(), tid, acct, UtilityReq{Plan: "founder", Host: subs.TerminalLabel(tid, acct) + ".ws.example.test"})
	if err != nil || ip != "10.1.2.3" {
		t.Fatalf("%q %v", ip, err)
	}
	for _, want := range [][]string{
		{"init shippin/dev-debian13 u-" + acct, "limits.cpu=1", "limits.memory=1GiB", "user.shippin.utility=" + acct},
		{"exec u-" + acct, "tee /etc/default/shippin-ttyd"},
		{"systemctl enable --now shippin-ttyd.service"},
		{"PUT", subs.TerminalLabel(tid, acct) + ".ws.example.test", `"@id":"` + utilityRouteID(tid, acct), "10.1.2.3:7681", "/auth/ws-check"},
	} {
		if f.find(want...) == nil {
			t.Fatalf("missing call %v", want)
		}
	}
	if _, err := s.StartUtility(context.Background(), tid, acct, UtilityReq{Plan: "founder", Host: "evil.example.com"}); err == nil {
		t.Fatal("host outside the ws base domain")
	}
}

func TestReadAIStatus(t *testing.T) {
	s := aiServer()
	useFake(t, fakeReply{match: "head -c", out: `{"version":2,"checked_at":"` + time.Now().UTC().Format(time.RFC3339) + `","providers":[{"provider":"claude","installed":true,"signed_in":true,"auth_known":true}]}`})
	st, err := s.readAIStatus(context.Background(), place{t: tid}, "u-x")
	if err != nil || len(st.Providers) != 1 || !st.Providers[0].SignedIn {
		t.Fatalf("%+v %v", st, err)
	}
	useFake(t, fakeReply{match: "head -c", out: strings.Repeat("x", 600)})
	if _, err := s.readAIStatus(context.Background(), place{t: tid}, "u-x"); err == nil {
		t.Fatal("oversized status accepted")
	}
}

func TestUtilityLogout(t *testing.T) {
	s := aiServer()
	f := useFake(t, append(aiFixtureReplies(), fakeReply{match: "/1.0/projects/t-", out: `{"config":{}}`})...)
	if err := s.UtilityLogout(context.Background(), tid, acct, "codex"); err == nil {
		t.Fatal("unconfigured provider")
	}
	if err := s.UtilityLogout(context.Background(), tid, acct, "claude"); err != nil {
		t.Fatal(err)
	}
	if f.find("exec u-"+acct, "--user 1000", "HOME=/home/dev", "bash -l -c", "claude auth logout") == nil {
		t.Fatal("vendor sign-out runs as the user")
	}
}

func TestAIRoutes(t *testing.T) {
	h := aiServer().Routes()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tenants/"+tid+"/utilities/nope/start", strings.NewReader("{}")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid account: %d", rec.Code)
	}
}
