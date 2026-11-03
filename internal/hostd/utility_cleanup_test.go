package hostd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"shippin.cloud/console/internal/subs"
)

func utilityReplies() []fakeReply {
	return append(aiFixtureReplies(),
		fakeReply{match: "/1.0/instances?recursion=1", out: `[{"name":"u-` + acct + `","config":{"user.shippin.utility":"` + acct + `","user.shippin.tenant":"` + tid + `"}}]`},
		fakeReply{match: "/1.0/projects/t-", out: `{"config":{}}`},
		fakeReply{match: "-X DELETE", out: "200"}, fakeReply{match: "-X GET", out: "404"},
		fakeReply{match: "/state", out: `{"status_code":102,"network":{"eth0":{"addresses":[{"family":"inet","address":"10.1.2.3","scope":"global"}]}}}`})
}

func TestStartUtilityCompensatesFailures(t *testing.T) {
	for _, phase := range []string{"start u-", "shippin-ws-init", "install -d", "tee /etc/default/",
		"systemctl enable", "systemctl restart", "-X PUT"} {
		t.Run(phase, func(t *testing.T) {
			f := useFake(t, append([]fakeReply{{match: phase, err: errors.New("injected failure")}}, utilityReplies()...)...)
			ip, err := aiServer().StartUtility(context.Background(), tid, acct,
				UtilityReq{Plan: "founder", Host: subs.TerminalLabel(tid, acct) + ".ws.example.test"})
			if err == nil || ip != "" || f.find("stop u-", "--force") == nil || f.find("-X GET") == nil {
				t.Fatalf("uncompensated failure: ip=%q err=%v", ip, err)
			}
			if phase != "-X PUT" && f.find("-X PUT") != nil {
				t.Fatal("published after failure")
			}
		})
	}
}

func TestStartUtilityCancellationStillCleansUp(t *testing.T) {
	f := useFake(t, utilityReplies()...)
	baseRunner := runner
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cleanupCalls := 0
	runner = func(c context.Context, stdin, bin string, args ...string) (string, error) {
		argv := strings.Join(args, " ")
		if strings.Contains(argv, "shippin-ws-init") {
			cancel()
			return "", context.Canceled
		}
		if ctx.Err() != nil {
			cleanupCalls++
			if c.Err() != nil {
				t.Fatal("cleanup inherited cancellation")
			}
		}
		return baseRunner(c, stdin, bin, args...)
	}
	_, err := aiServer().StartUtility(ctx, tid, acct,
		UtilityReq{Plan: "founder", Host: subs.TerminalLabel(tid, acct) + ".ws.example.test"})
	if !errors.Is(err, context.Canceled) || cleanupCalls < 4 || f.find("stop u-") == nil {
		t.Fatalf("cleanup missing: %v, calls=%d", err, cleanupCalls)
	}
}

func TestUtilityRemovalMustBeConfirmed(t *testing.T) {
	for _, tc := range []struct {
		method, code string
		err          error
	}{
		{"DELETE", "", errors.New("edge unreachable")}, {"DELETE", "500", nil},
		{"DELETE", "", nil}, {"GET", "200", nil}, {"GET", "404", errors.New("transport error")},
	} {
		t.Run(tc.method+tc.code, func(t *testing.T) {
			f := useFake(t, append([]fakeReply{{match: "-X " + tc.method, out: tc.code, err: tc.err}}, utilityReplies()...)...)
			if err := aiServer().StopUtility(context.Background(), tid, acct); err == nil {
				t.Fatal("unconfirmed removal accepted")
			}
			if f.find("stop u-", "--force") == nil {
				t.Fatal("route failure prevented container stop")
			}
			if _, err := aiServer().StartUtility(context.Background(), tid, acct,
				UtilityReq{Plan: "founder", Host: subs.TerminalLabel(tid, acct) + ".ws.example.test"}); err == nil {
				t.Fatal("started with uncertain old route")
			}
			if f.find("start u-") != nil || f.find("-X PUT") != nil {
				t.Fatal("runner started or published")
			}
		})
	}
}

func TestStopUtilityRequiresStoppedState(t *testing.T) {
	useFake(t, append([]fakeReply{{match: "/state", out: `{"status_code":103}`}}, utilityReplies()...)...)
	if err := aiServer().StopUtility(context.Background(), tid, acct); err == nil {
		t.Fatal("running accepted as stopped")
	}
	useFake(t, append([]fakeReply{{match: "stop u-", err: errors.New("already stopped")}}, utilityReplies()...)...)
	if err := aiServer().StopUtility(context.Background(), tid, acct); err != nil {
		t.Fatal(err)
	}
}
