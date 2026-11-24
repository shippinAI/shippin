package jobs

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

// testJobs opens SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated).
func testJobs(t *testing.T) (*Store, *db.DB, string) {
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
	tid := uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'t','p')`, tid, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	return NewStore(d.Pool), d, tid
}

func job(tid, target string) Job {
	return Job{TenantID: tid, Kind: KindWorkspaceProvision, TargetType: "workspace", TargetID: target, MaxAttempts: 2, CreatedBy: "u"}
}

func TestEnqueueJoinsLiveJob(t *testing.T) {
	s, _, tid := testJobs(t)
	ctx := context.Background()
	a, created, err := s.Enqueue(ctx, job(tid, "w1"))
	if err != nil || !created || a.Status != Queued {
		t.Fatalf("first enqueue: %v %v %+v", err, created, a)
	}
	b, created, err := s.Enqueue(ctx, job(tid, "w1"))
	if err != nil || created || b.ID != a.ID {
		t.Fatalf("second enqueue should join %s: %v %v %+v", a.ID, err, created, b)
	}
	c, _ := s.Claim(ctx, "o", []string{KindWorkspaceProvision}, time.Minute)
	if err := s.Succeed(ctx, c.ID, "o", map[string]string{"ip": "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	d, created, err := s.Enqueue(ctx, job(tid, "w1"))
	if err != nil || !created || d.ID == a.ID {
		t.Fatalf("after success a new job is expected: %v %v", err, created)
	}
}

func TestConcurrentClaimsNeverShare(t *testing.T) {
	s, _, tid := testJobs(t)
	ctx := context.Background()
	for i := range 20 {
		if _, _, err := s.Enqueue(ctx, job(tid, uuid.NewString()[:8]+string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	seen := map[string]string{}
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			for {
				j, err := s.Claim(ctx, owner, []string{KindWorkspaceProvision}, time.Minute)
				if err != nil || j == nil {
					return
				}
				mu.Lock()
				if prev, dup := seen[j.ID]; dup {
					t.Errorf("job %s claimed by %s and %s", j.ID, prev, owner)
				}
				seen[j.ID] = owner
				mu.Unlock()
			}
		}(string(rune('A' + w)))
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Fatalf("claimed %d of 20", len(seen))
	}
}

func TestExpiredLeaseIsReclaimed(t *testing.T) {
	s, d, tid := testJobs(t)
	ctx := context.Background()
	j, _, _ := s.Enqueue(ctx, job(tid, "w2"))
	if c, _ := s.Claim(ctx, "dead", []string{KindWorkspaceProvision}, time.Minute); c == nil || c.ID != j.ID {
		t.Fatal("first claim")
	}
	if c, _ := s.Claim(ctx, "live", []string{KindWorkspaceProvision}, time.Minute); c != nil {
		t.Fatal("a leased job was claimed twice")
	}
	_, _ = d.Pool.Exec(ctx, `UPDATE jobs SET lease_until = now() - interval '1 second' WHERE id=$1`, j.ID)
	c, _ := s.Claim(ctx, "live", []string{KindWorkspaceProvision}, time.Minute)
	if c == nil || c.Attempts != 2 {
		t.Fatalf("expired lease not reclaimed: %+v", c)
	}
	if err := s.Heartbeat(ctx, j.ID, "dead", time.Minute); err != ErrLostLease {
		t.Fatalf("old owner kept the lease: %v", err)
	}
	if err := s.Succeed(ctx, j.ID, "dead", nil); err != ErrLostLease {
		t.Fatalf("old owner finished the job: %v", err)
	}
}

func TestFailRetriesThenFails(t *testing.T) {
	s, _, tid := testJobs(t)
	ctx := context.Background()
	j, _, _ := s.Enqueue(ctx, job(tid, "w3"))
	_, _ = s.Claim(ctx, "o", []string{KindWorkspaceProvision}, time.Minute)
	if final, err := s.Fail(ctx, j.ID, "o", "boom", true, 0); err != nil || final {
		t.Fatalf("attempt 1 should requeue: %v %v", final, err)
	}
	if c, _ := s.Claim(ctx, "o", []string{KindWorkspaceProvision}, time.Minute); c == nil {
		t.Fatal("requeued job not claimable")
	}
	if final, err := s.Fail(ctx, j.ID, "o", "boom again", true, 0); err != nil || !final {
		t.Fatalf("attempt 2 of 2 should fail: %v %v", final, err)
	}
	g, _ := s.Get(ctx, j.ID)
	if g.Status != Failed || g.Error != "boom again" || g.FinishedAt == nil {
		t.Fatalf("got %+v", g)
	}
	if _, err := s.Get(ctx, "not-a-uuid"); err != ErrNotFound {
		t.Fatalf("bad id: %v", err)
	}
}
