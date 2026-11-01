package hostd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const wid = "019a0000-0000-7000-8000-00000000a001"

func siteFake(t *testing.T) *fakeRunner {
	return useFake(t,
		fakeReply{match: "/snapshots?recursion=1", out: `[{"name":"pub-200","created_at":"2026-10-19T10:00:00Z"},
			{"name":"daily","created_at":"2026-10-19T11:00:00Z"},{"name":"pub-100","created_at":"2026-10-18T10:00:00Z"}]`},
		fakeReply{match: "/state", out: `{"network":{"eth0":{"addresses":[{"family":"inet","address":"10.128.4.9","scope":"global"}]}}}`},
		fakeReply{match: "query /1.0/instances/s-", out: `{"devices":{"ssh":{"type":"proxy"},"root":{"type":"disk"}}}`},
		fakeReply{match: "user.shippin.site_release", out: "pub-200\n"},
	)
}

func TestSiteHost(t *testing.T) {
	s := testServer()
	if h, err := s.SiteHost("my-app"); err != nil || h != "my-app--site.ws.example.test" {
		t.Fatalf("%q %v", h, err)
	}
	for _, bad := range []string{"", "-x", "A", "a.b", strings.Repeat("a", 33)} {
		if _, err := s.SiteHost(bad); err == nil {
			t.Errorf("slug %q accepted", bad)
		}
	}
}

func TestPublishSnapshotsCopiesAndRoutesPublic(t *testing.T) {
	f := siteFake(t)
	s := testServer()
	rel, err := s.Publish(context.Background(), place{t: tid}, wid, "a--site.ws.example.test", 5173, time.Unix(300, 0))
	if err != nil || rel != "pub-300" {
		t.Fatalf("%q %v", rel, err)
	}
	if f.find("snapshot create w-"+wid+" pub-300") == nil || f.find("copy w-"+wid+"/pub-300 s-"+wid) == nil {
		t.Fatal("no snapshot+copy")
	}
	if f.find("device remove s-"+wid+" ssh") == nil {
		t.Fatal("inherited proxy device kept on the site instance")
	}
	put := f.find("PUT", "routes/0")
	if put == nil || !strings.Contains(put[len(put)-1], `"site-`+wid+`"`) || strings.Contains(put[len(put)-1], "ws-check") {
		t.Fatalf("route must be public site-<w>: %v", put)
	}
	if !strings.Contains(put[len(put)-1], "10.128.4.9:5173") {
		t.Fatal("route upstream")
	}
	if _, err := s.Publish(context.Background(), place{t: tid}, wid, "h", 3000, time.Now()); err == nil {
		t.Fatal("IDE port published")
	}
}

func TestRollbackPicksPreviousAndRejectsUnknown(t *testing.T) {
	f := siteFake(t)
	s := testServer()
	rel, err := s.Rollback(context.Background(), place{t: tid}, wid, "a--site.ws.example.test", "", "pub-200", 8080)
	if err != nil || rel != "pub-100" || f.find("copy w-"+wid+"/pub-100") == nil {
		t.Fatalf("%q %v", rel, err)
	}
	if _, err := s.Rollback(context.Background(), place{t: tid}, wid, "h", "", "pub-100", 8080); err == nil {
		t.Fatal("rollback past the oldest release")
	}
	if _, err := s.Rollback(context.Background(), place{t: tid}, wid, "h", "daily", "", 8080); err == nil {
		t.Fatal("non-release snapshot accepted")
	}
}

func TestSiteHandlerUnpublishAndState(t *testing.T) {
	f := siteFake(t)
	h := testServer().Routes()
	body := `{"action":"unpublish","slug":"a"}`
	rq := httptest.NewRequest("POST", "/tenants/"+tid+"/workspaces/"+wid+"/site", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, rq)
	if rr.Code != http.StatusOK || f.find("DELETE", "/id/site-"+wid) == nil || f.find("stop --force s-"+wid) == nil {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
	if !strings.Contains(rr.Body.String(), `"current":"pub-200"`) || !strings.Contains(rr.Body.String(), `"rrflow_recorded":true`) {
		t.Fatalf("state %s", rr.Body)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tenants/"+tid+"/workspaces/"+wid+"/site", strings.NewReader(`{"action":"nuke","slug":"a"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad action %d", rr.Code)
	}
}

func TestReleasesNewestFirstOnlyPub(t *testing.T) {
	rs := releases(snapList{{Name: "pub-1", CreatedAt: time.Unix(1, 0)}, {Name: "x"}, {Name: "pub-9", CreatedAt: time.Unix(9, 0)}})
	if len(rs) != 2 || rs[0].ID != "pub-9" {
		t.Fatalf("%v", rs)
	}
}
