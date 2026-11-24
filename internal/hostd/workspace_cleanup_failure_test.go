package hostd

import (
	"context"
	"errors"
	"strings"
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

// A published site is a copy of the workspace: removing the workspace removes the site's public
// route and its instance, and the home volume only after both instances are gone.
func TestRemoveTakesThePublishedSiteWithIt(t *testing.T) {
	f := useFake(t,
		fakeReply{match: "-X GET", out: `[{"@id":"site-one"},{"@id":"site-other"}]`},
		fakeReply{match: "/1.0/network-acls/", out: `{"ingress":[]}`},
		fakeReply{match: "/1.0/instances?recursion=1", out: `[{"name":"w-one"},{"name":"s-one"},{"name":"s-other"}]`},
		fakeReply{match: "volumes/custom?recursion=1", out: `[{"name":"` + HomeVolume("one") + `"}]`},
	)
	s := testServer()
	s.cfg.StateDir = t.TempDir() // remove rewrites the shared-port file
	if err := s.remove(context.Background(), place{t: "t"}, "one"); err != nil {
		t.Fatal(err)
	}
	if f.find("DELETE", "/id/site-one") == nil || f.find("delete", "s-one", "--force") == nil {
		t.Fatal("the published site outlived its workspace")
	}
	if f.find("DELETE", "/id/site-other") != nil || f.find("delete", "s-other") != nil {
		t.Fatal("another workspace's site was removed")
	}
	site, vol := -1, -1
	for i, c := range f.calls {
		switch j := strings.Join(c, " "); {
		case strings.Contains(j, "delete s-one"):
			site = i
		case strings.Contains(j, "volume delete"):
			vol = i
		}
	}
	if site < 0 || vol < site {
		t.Fatalf("home volume deleted before the site instance (site=%d volume=%d)", site, vol)
	}
}
