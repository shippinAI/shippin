package servers

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

// Needs SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated).
func testStore(t *testing.T) (*Store, context.Context, string) {
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
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'operator','founder')`, tid, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	return &Store{Pool: d.Pool, Clock: Clock{Interval: 15 * time.Second, Missed: 3}}, ctx, tid
}

// TestEnrollmentLifecycle: pending → token → enroll once → beats → stale → revoked; secrets are
// only ever stored hashed.
func TestEnrollmentLifecycle(t *testing.T) {
	s, ctx, tid := testStore(t)
	g, err := s.Create(ctx, tid, "edge", "Edge", "edge", "u")
	if err != nil || g.State != Pending {
		t.Fatalf("create: %+v %v", g, err)
	}
	if _, err := s.Create(ctx, tid, "edge", "Edge 2", "edge", "u"); err != ErrSlugTaken {
		t.Fatalf("dup slug: %v", err)
	}
	old, _ := s.IssueToken(ctx, tid, g.ID, "u", time.Hour)
	tok, _ := s.IssueToken(ctx, tid, g.ID, "u", time.Hour)
	if _, err := s.Enroll(ctx, old.Token, "h", "v", time.Minute); err != ErrBadToken {
		t.Fatalf("a re-issue must void the old token: %v", err)
	}
	var clear int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_enroll_tokens WHERE token_sha256 IN ($1,$2)`, tok.Token, old.Token).Scan(&clear)
	if clear != 0 {
		t.Fatal("a token was stored in clear")
	}
	var wg sync.WaitGroup
	wins := make(chan *Enrolled, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e, err := s.Enroll(ctx, tok.Token, "edge", "1", time.Minute); err == nil {
				wins <- e
			}
		}()
	}
	wg.Wait()
	close(wins)
	if len(wins) != 1 {
		t.Fatalf("token redeemed %d times, want exactly once", len(wins))
	}
	e := <-wins
	if e.ServerID != g.ID || e.HeartbeatSeconds != 15 {
		t.Fatalf("enrolled: %+v", e)
	}
	got, _ := s.Get(ctx, tid, g.ID)
	if got.State != Active || got.Hostname != "edge" {
		t.Fatalf("after enroll: %+v", got)
	}
	identity, err := s.Identity(ctx, g.ID)
	if err != nil || identity.HostnameSource != "hostd_enrollment" || identity.HostnameObserved == nil {
		t.Fatalf("hostname provenance after enroll: %+v %v", identity, err)
	}
	start := time.Now()
	s.Now = func() time.Time { return start.Add(46 * time.Second) }
	if got, _ = s.Get(ctx, tid, g.ID); got.State != Stale {
		t.Fatalf("3 missed beats must read stale, got %s", got.State)
	}
	if _, _, err := s.Beat(ctx, e.Credential, "2", time.Minute); err != nil {
		t.Fatalf("beat: %v", err)
	}
	if got, _ = s.Get(ctx, tid, g.ID); got.State != Active || got.AgentVersion != "2" {
		t.Fatalf("a beat must bring it back: %+v", got)
	}
	s.Now = func() time.Time { return start.Add(5 * time.Minute) }
	if _, _, err := s.Beat(ctx, e.Credential, "2", time.Minute); err != ErrBadCred {
		t.Fatalf("an expired credential must be refused: %v", err)
	}
	s.Now = nil
	if err := s.Revoke(ctx, tid, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, tid, g.ID); err != ErrState {
		t.Fatalf("revoke twice: %v", err)
	}
	if _, err := s.IssueToken(ctx, tid, g.ID, "u", time.Hour); err != ErrState {
		t.Fatalf("token after revoke: %v", err)
	}
	if _, err := s.Get(ctx, uuid.NewString(), g.ID); err != ErrNotFound {
		t.Fatalf("other tenant sees the server: %v", err)
	}
}

// TestExpiredToken: a token past its TTL can't be redeemed.
func TestExpiredToken(t *testing.T) {
	s, ctx, tid := testStore(t)
	g, _ := s.Create(ctx, tid, "edge", "Edge", "edge", "u")
	tok, _ := s.IssueToken(ctx, tid, g.ID, "u", time.Minute)
	s.Now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := s.Enroll(ctx, tok.Token, "h", "v", time.Minute); err != ErrBadToken {
		t.Fatalf("expired token: %v", err)
	}
}

func TestHypervisorColumns(t *testing.T) {
	s, ctx, tid := testStore(t)
	g, err := s.Create(ctx, tid, "pve", "Cluster", "lab", "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Endpoint(ctx, g.ID); err != ErrNotFound {
		t.Fatalf("unset endpoint: %v", err)
	}
	ep := Endpoint{ID: g.ID, Kind: "proxmox", BaseURL: "https://pve.example:8006", Node: "n1", TokenID: "agent@pve!console", TokenEnv: "SHIPPIN_HV_TOKEN"}
	if err := s.SetEndpoint(ctx, ep); err != nil {
		t.Fatal(err)
	}
	got, err := s.Endpoint(ctx, g.ID)
	if err != nil || got.Kind != "proxmox" || got.TokenEnv != "SHIPPIN_HV_TOKEN" || got.Slug != "pve" || got.AllowTenantDeploymentSecrets || got.Revision != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	ep.AllowTenantDeploymentSecrets = true
	ep.Kind = "incus"
	if err := s.SetEndpoint(ctx, ep); err != nil {
		t.Fatal(err)
	}
	got, err = s.Endpoint(ctx, g.ID)
	if err != nil || !got.AllowTenantDeploymentSecrets || got.Revision != 3 {
		t.Fatalf("explicit Incus secret trust setting was not persisted: %+v %v", got, err)
	}
	ep.Kind = "proxmox"
	if err := s.SetEndpoint(ctx, ep); err != nil {
		t.Fatal(err)
	}
	got, err = s.Endpoint(ctx, g.ID)
	if err != nil || got.AllowTenantDeploymentSecrets || got.Revision != 4 {
		t.Fatalf("provider change must clear Incus-only trust setting: %+v %v", got, err)
	}
	all, err := s.Endpoints(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("%v %v", all, err)
	}
	vis, err := s.VisibleEndpoint(ctx, tid, g.ID)
	if err != nil || vis.ID != g.ID {
		t.Fatalf("owner visibility %+v %v", vis, err)
	}
	if _, err := s.VisibleEndpoint(ctx, uuid.NewString(), g.ID); err != ErrNotFound {
		t.Fatalf("another org must not see the server: %v", err)
	}
	if err := s.SetEndpoint(ctx, Endpoint{ID: g.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Endpoint(ctx, g.ID); err != ErrNotFound {
		t.Fatalf("detached endpoint: %v", err)
	}
	var revision int64
	if err := s.Pool.QueryRow(ctx, `SELECT hv_revision FROM servers WHERE id=$1::uuid`, g.ID).Scan(&revision); err != nil || revision != 5 {
		t.Fatalf("detach revision=%d err=%v", revision, err)
	}
}

func TestSystemIdentityIsExplicitAndUnique(t *testing.T) {
	s, ctx, tid := testStore(t)
	a, err := s.Create(ctx, tid, "identity-a", "Primary", "lab", "u")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(ctx, tid, "identity-b", "Secondary", "lab", "u")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.SetIdentity(ctx, SystemIdentity{ServerID: a.ID, Label: "Edge Primary", Aliases: []string{"edge-a"}, Tags: []string{"production"}, AgentHubTenantID: "tenant-external", AgentHubMachineID: "machine-external"}, "operator")
	if err != nil || got.ServerID != a.ID || got.Label != "Edge Primary" || got.HostnameSource != "unknown" || got.UpdatedBy != "operator" || got.UpdatedAt == nil || len(got.Aliases) != 1 || got.Aliases[0] != "edge-a" {
		t.Fatalf("identity save: %+v %v", got, err)
	}
	if _, err = s.SetIdentity(ctx, SystemIdentity{ServerID: b.ID, Label: "Secondary", AgentHubTenantID: "tenant-external", AgentHubMachineID: "machine-external"}, "operator"); err != ErrIdentityConflict {
		t.Fatalf("duplicate AgentHub mapping must fail: %v", err)
	}
	if _, err = s.SetIdentity(ctx, SystemIdentity{ServerID: a.ID, Label: "Primary", AgentHubTenantID: "tenant-only"}, "operator"); err != ErrIdentityInvalid {
		t.Fatalf("half mapping must fail: %v", err)
	}
	if _, err = s.SetIdentity(ctx, SystemIdentity{ServerID: a.ID, Label: "Primary", Aliases: []string{"Edge-A", "edge-a"}}, "operator"); err != ErrIdentityInvalid {
		t.Fatalf("case-insensitive duplicate aliases must fail: %v", err)
	}
	if _, err = s.SetIdentity(ctx, SystemIdentity{ServerID: uuid.NewString(), Label: "Unknown", Aliases: []string{"unused"}}, "operator"); err != ErrNotFound {
		t.Fatalf("unknown system must fail: %v", err)
	}
}

// A platform host belongs to the system tenant. A customer sees it only if it is assigned to them, and
// then only to read. Nothing is seeded: a fresh install has no hosts at all.
func TestCustomersSeeOnlyTheirOwnServersAndWhatIsAssignedToThem(t *testing.T) {
	s, ctx, tid := testStore(t)
	if refs, err := s.Registry(ctx); err != nil || len(refs) != 0 {
		t.Fatalf("a fresh install must not come with anyone's hosts: %+v %v", refs, err)
	}
	host, err := s.CreateEndpoint(ctx, Endpoint{ID: uuid.NewString(), Slug: "platform-1", Label: "Platform 1", Site: "main", Kind: "incus", BaseURL: "https://incus.invalid:8443"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if list, err := s.List(ctx, tid); err != nil || len(list) != 0 {
		t.Fatalf("a platform host must be invisible to a customer: %+v %v", list, err)
	}
	if _, err := s.Get(ctx, tid, host.ID); err != ErrNotFound {
		t.Fatalf("and unreadable by id: %v", err)
	}
	if _, err := s.IssueToken(ctx, tid, host.ID, "u", time.Hour); err != ErrNotFound {
		t.Fatalf("and unknown to enrollment: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO server_assignments(server_id, tenant_id, role, created_by) VALUES($1,$2,'placement','u')`, host.ID, tid); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, tid)
	if err != nil || len(list) != 1 || !list[0].Estate {
		t.Fatalf("once assigned it is visible and marked: %+v %v", list, err)
	}
	if _, err := s.IssueToken(ctx, tid, host.ID, "u", time.Hour); err != ErrReadOnly {
		t.Fatalf("and read-only: %v", err)
	}
	var owner string
	if err := s.Pool.QueryRow(ctx, `SELECT tenant_id::text FROM servers WHERE id=$1`, host.ID).Scan(&owner); err != nil || owner != SystemTenantID {
		t.Fatalf("the system tenant owns it: %q %v", owner, err)
	}
}
