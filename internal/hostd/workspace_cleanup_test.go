package hostd

import (
	"context"
	"errors"
	"testing"
)

func TestWorkspaceRouteCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		err       error
		fail      bool
	}{
		{"absent", "[]", nil, false}, {"unavailable", "", errors.New("offline"), true}, {"malformed", "broken", nil, true},
		{"owned only", `[{"@id":"ws-one"},{"@id":"pv-one-3000"},{"@id":"ws-other"}]`, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := useFake(t, fakeReply{match: "-X GET", out: tc.raw, err: tc.err})
			err := testServer().removeWorkspaceRoutes(context.Background(), "one")
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v", err)
			}
			if f.find("DELETE", "/id/ws-other") != nil {
				t.Fatal("removed foreign route")
			}
			if tc.name == "owned only" && (f.find("DELETE", "/id/ws-one") == nil || f.find("DELETE", "/id/pv-one-3000") == nil) {
				t.Fatal("owned routes not removed")
			}
		})
	}
}
