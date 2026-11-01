package hostd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"shippin.cloud/console/internal/subs"
)

func TestStartUtilityWithdrawsRouteBeforeEarlyFailure(t *testing.T) {
	for _, phase := range []string{"/1.0/projects/t-", "/1.0/instances?recursion=1", "/volumes/custom/ai-"} {
		t.Run(phase, func(t *testing.T) {
			f := useFake(t, utilityReplies()...)
			baseRunner := runner
			routed := true // an already-running terminal has a preexisting route
			runner = func(ctx context.Context, input, bin string, args ...string) (string, error) {
				argv := strings.Join(args, " ")
				if strings.Contains(argv, "-X DELETE") {
					routed = false
				}
				if strings.Contains(argv, phase) {
					if routed {
						t.Fatal("discovery or mount happened before withdrawal")
					}
					return "", errors.New("injected early failure")
				}
				return baseRunner(ctx, input, bin, args...)
			}
			_, err := aiServer().StartUtility(context.Background(), tid, acct,
				UtilityReq{Plan: "founder", Host: subs.TerminalLabel(tid, acct) + ".ws.example.test"})
			if err == nil || routed || f.find("-X PUT") != nil || f.find("init ") != nil {
				t.Fatal("unsafe early failure")
			}
			if phase == "/volumes/custom/ai-" && f.find("stop u-") == nil {
				t.Fatal("verified owner not stopped after mount failure")
			}
		})
	}
}

func TestUtilityInventoryRequiresConfirmedAbsenceAndOwner(t *testing.T) {
	for _, inventory := range []string{"", "null", "{}", "[null]", "[{}]", `[{"name":"other","config":{}},null]`,
		`[{"name":"other\u00a0name","config":{}}]`, `[{"name":"other\fname","config":{}}]`,
		`[{"name":"other\bname","config":{}}]`,
		`[{"name":".","config":{}}]`, `[{"name":"..","config":{}}]`,
		`[{"name":"-other","config":{}}]`, `[{"name":"other-","config":{}}]`,
		`[{"name":"1other","config":{}}]`, `[{"name":"other_name","config":{}}]`,
		`[{"name":"other.name","config":{}}]`, `[{"name":"other:name","config":{}}]`,
		`[{"name":"u-` + acct + `","config":{"user.shippin.utility":"other"}}]`} {
		t.Run(inventory, func(t *testing.T) {
			f := useFake(t, append([]fakeReply{{match: "/1.0/instances?recursion=1", out: inventory}}, utilityReplies()...)...)
			_, err := aiServer().StartUtility(context.Background(), tid, acct,
				UtilityReq{Plan: "founder", Host: subs.TerminalLabel(tid, acct) + ".ws.example.test"})
			if err == nil || f.find("init ") != nil || f.find("storage volume") != nil {
				t.Fatal("unverified inventory treated as absence")
			}
			if err := aiServer().StopUtility(context.Background(), tid, acct); err == nil {
				t.Fatal("unverified owner accepted")
			}
			if f.find("stop u-") != nil {
				t.Fatal("stopped unverified instance")
			}
		})
	}
	useFake(t, append([]fakeReply{{match: "/1.0/instances?recursion=1", out: "[]"}}, utilityReplies()...)...)
	if err := aiServer().StopUtility(context.Background(), tid, acct); err != nil {
		t.Fatal(err)
	}
}

func TestUtilityInventoryDocumentedNames(t *testing.T) {
	for _, name := range []string{"a", "Z", "a1", "a-b", "A--B0", strings.Repeat("a", 63), UtilityInstance(acct)} {
		if !utilityInventoryName.MatchString(name) {
			t.Errorf("rejected documented name %q", name)
		}
	}
	for _, name := range []string{"", "1", "1a", "a-", "a_b", "a.b", "é", strings.Repeat("a", 64)} {
		if utilityInventoryName.MatchString(name) {
			t.Errorf("accepted undocumented name %q", name)
		}
	}
}
