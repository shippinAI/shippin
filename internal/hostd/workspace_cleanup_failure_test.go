package hostd

import (
	"context"
	"errors"
	"testing"
)

func TestCleanupRequiresPositiveAbsence(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		fail      bool
	}{{"empty", "[]", false}, {"present", `[{"name":"w-one"}]`, false}, {"null", "null", true}, {"malformed", "broken", true}} {
		t.Run(tc.name, func(t *testing.T) {
			useFake(t, fakeReply{match: "query", out: tc.raw})
			present, err := (place{t: "t"}).cleanupContains(context.Background(), "/1.0/instances?recursion=1", "w-one")
			if (err != nil) != tc.fail || present != (tc.name == "present") {
				t.Fatalf("present=%v err=%v", present, err)
			}
		})
	}
	t.Run("offline", func(t *testing.T) {
		useFake(t, fakeReply{match: "query", err: errors.New("offline")})
		if _, err := (place{t: "t"}).cleanupContains(context.Background(), "/1.0/instances?recursion=1", "w-one"); err == nil {
			t.Fatal("offline treated as absent")
		}
	})
}
func TestRouteCleanupFailureStopsDeletion(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		delErr    error
	}{{"null", "null", nil}, {"delete refused", `[{"@id":"ws-one"}]`, errors.New("offline")}} {
		t.Run(tc.name, func(t *testing.T) {
			f := useFake(t, fakeReply{match: "-X GET", out: tc.raw}, fakeReply{match: "-X DELETE", err: tc.delErr})
			if err := testServer().remove(context.Background(), place{t: "t"}, "one"); err == nil {
				t.Fatal("cleanup failed open")
			}
			if f.find("delete", "w-one") != nil || f.find("volume", "delete") != nil || f.find("query", "/1.0/instances") != nil {
				t.Fatal("instance/volume touched after route failure")
			}
		})
	}
}
