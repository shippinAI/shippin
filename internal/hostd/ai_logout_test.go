package hostd

import (
	"context"
	"strings"
	"testing"
)

func TestUtilityLogoutRequiresVerifiedHome(t *testing.T) {
	for _, tc := range []struct {
		name     string
		index    int
		old, new string
	}{
		{"owner", 0, acct, "wrong"}, {"sharing", 0, "/instances/u-", "/instances/w-"},
		{"source", 1, `"source":"ai-`, `"source":"wrong-`}, {"path", 1, "/home/dev/.ai", "/tmp"},
		{"device", 1, `"ai-home":`, `"other":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := aiFixtureReplies()
			r[tc.index].out = strings.ReplaceAll(r[tc.index].out, tc.old, tc.new)
			r = append(r, fakeReply{match: "/1.0/projects/t-", out: `{"config":{}}`})
			f := useFake(t, r...)
			if err := aiServer().UtilityLogout(context.Background(), tid, acct, "claude"); err == nil {
				t.Fatal("unsafe logout")
			}
			if f.find("exec u-"+acct) != nil {
				t.Fatal("credential command ran before isolation check")
			}
		})
	}
}
