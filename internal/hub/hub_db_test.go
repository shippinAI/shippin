package hub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/db"
)

// testStore opens SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated).
func testStore(t *testing.T) *Store {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	d.Pool.Close()
	if d, err = db.Open(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	return &Store{Pool: d.Pool}
}

func TestLifecycleLeaseAndApproval(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	task, err := s.Create(ctx, "org1", "Ship the hub", "", "operator")
	if err != nil || task.Status != Open {
		t.Fatalf("create: %v %+v", err, task)
	}
	if _, err := s.Claim(ctx, "org2", task.ID, "a1", time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other org must not see the task: %v", err)
	}
	if task, err = s.Claim(ctx, "org1", task.ID, "a1", time.Minute); err != nil || task.LeaseOwner != "a1" {
		t.Fatalf("claim: %v", err)
	}
	if _, err := s.Claim(ctx, "org1", task.ID, "a2", time.Minute); !errors.Is(err, ErrConflict) {
		t.Fatalf("a live lease must block a second agent: %v", err)
	}
	if _, err := s.Progress(ctx, "org1", task.ID, "a2", "x", time.Minute); !errors.Is(err, ErrConflict) {
		t.Fatalf("non-holder progress: %v", err)
	}
	if task, err = s.Progress(ctx, "org1", task.ID, "a1", "half way", time.Minute); err != nil || task.Status != InProgress {
		t.Fatalf("progress: %v", err)
	}
	if _, err := s.Decide(ctx, "org1", task.ID, "operator", true, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("approve before request: %v", err)
	}
	if task, err = s.RequestApproval(ctx, "org1", task.ID, "a1", "please sign off"); err != nil || task.Status != NeedsApproval {
		t.Fatalf("request: %v", err)
	}
	if task, err = s.Decide(ctx, "org1", task.ID, "operator", true, ""); err != nil || task.Status != Approved || task.DecidedBy != "operator" {
		t.Fatalf("approve: %v %+v", err, task)
	}
	if task, err = s.Finish(ctx, "org1", task.ID, "a1"); err != nil || task.Status != Done {
		t.Fatalf("finish: %v", err)
	}
}

func TestExpiredLeaseCanBeReclaimed(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	task, _ := s.Create(ctx, "o", "t", "", "j")
	if _, err := s.Claim(ctx, "o", task.ID, "a1", -time.Second); err != nil {
		t.Fatal(err)
	}
	if task, err := s.Claim(ctx, "o", task.ID, "a2", time.Minute); err != nil || task.LeaseOwner != "a2" {
		t.Fatalf("reclaim after expiry: %v", err)
	}
}

func TestDecisionIsOwnerOnly(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	task, _ := s.Create(ctx, "o", "t", "", "j")
	_, _ = s.Claim(ctx, "o", task.ID, "agent", time.Minute)
	_, _ = s.RequestApproval(ctx, "o", task.ID, "agent", "")
	pol := &approval.Policy{Enabled: true, Rules: []approval.Rule{{ID: "review", Actions: []string{"hub.decide"}, ApproverSubs: []string{"owner"}}}}
	h := &Handler{Store: s, Policy: pol, Lease: time.Minute, Who: func(w http.ResponseWriter, r *http.Request) (*Principal, bool) {
		return &Principal{Sub: r.Header.Get("X-Sub"), OrgID: "o"}, true
	}}
	mux := http.NewServeMux()
	h.Register(mux)
	for sub, want := range map[string]int{"agent": 403, "": 403, "owner": 200} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/hub/tasks/"+task.ID+"/decision", strings.NewReader(`{"approve":true}`))
		req.Header.Set("X-Sub", sub)
		mux.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%q: got %d want %d", sub, rec.Code, want)
		}
	}
}

func TestMissingPolicyApprovesNobody(t *testing.T) {
	if (&Handler{}).IsOwner(&Principal{Sub: "configured-by-environment"}) {
		t.Fatal("owner identity without an explicit policy must fail closed")
	}
}

func TestAnApprovalIsSpentOnce(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	first, err := s.UpsertApproval(ctx, "org", "hv:s1:g1:stop", "stop g1", "", "ada")
	if err != nil || first.Status != "needs_approval" {
		t.Fatalf("%+v %v", first, err)
	}
	if used, err := s.UseApproval(ctx, "org", "hv:s1:g1:stop"); err != nil || used {
		t.Fatalf("an undecided card must not be spendable: %v %v", used, err)
	}
	if _, err := s.Decide(ctx, "org", first.ID, "owner", true, ""); err != nil {
		t.Fatal(err)
	}
	if used, err := s.UseApproval(ctx, "org", "hv:s1:g1:stop"); err != nil || !used {
		t.Fatalf("the approved card must be spendable: %v %v", used, err)
	}
	if used, err := s.UseApproval(ctx, "org", "hv:s1:g1:stop"); err != nil || used {
		t.Fatalf("it must be spendable once: %v %v", used, err)
	}
	next, err := s.UpsertApproval(ctx, "org", "hv:s1:g1:stop", "stop g1", "", "ada")
	if err != nil || next.ID == first.ID || next.Status != "needs_approval" {
		t.Fatalf("the next change must wait on a new card: %+v %v", next, err)
	}
}
