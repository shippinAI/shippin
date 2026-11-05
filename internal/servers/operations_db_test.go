package servers

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOperationsAreDurableAndARestartSettlesThem(t *testing.T) {
	s, ctx, tenant := testStore(t)
	g, err := s.Create(ctx, tenant, "ops-host", "Ops", "lab", "u")
	if err != nil {
		t.Fatal(err)
	}
	done, err := s.StartOperation(ctx, Operation{ServerID: g.ID, Guest: "a", Verb: "stop", Actor: "u", ActorName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishOperation(ctx, done, OpSucceeded, "", "stopped"); err != nil {
		t.Fatal(err)
	}
	// A final state is never overwritten.
	if err := s.FinishOperation(ctx, done, OpFailed, "late", ""); err != nil {
		t.Fatal(err)
	}
	running, _ := s.StartOperation(ctx, Operation{ServerID: g.ID, Guest: "b", Verb: "stop", Actor: "u"})
	batch := uuid.NewString()
	queued, _ := s.StartOperation(ctx, Operation{ServerID: g.ID, BatchID: batch, Guest: "c", Verb: "shutdown", Status: OpQueued, Actor: "u"})

	n, err := s.AbandonOperations(ctx)
	if err != nil || n != 2 {
		t.Fatalf("abandoned %d, %v", n, err)
	}
	ops, err := s.Operations(ctx, g.ID, 10)
	if err != nil || len(ops) != 3 {
		t.Fatalf("%d %v", len(ops), err)
	}
	byID := map[string]Operation{}
	for _, o := range ops {
		byID[o.ID] = o
	}
	if o := byID[done]; o.Status != OpSucceeded || o.GuestStatus != "stopped" || o.ActorName != "Ada" || o.ServerLabel != "Ops" || o.FinishedAt == nil {
		t.Fatalf("%+v", o)
	}
	for _, id := range []string{running, queued} {
		if o := byID[id]; o.Status != OpUnknown || !strings.Contains(o.Detail, "restarted") {
			t.Fatalf("%+v", o)
		}
	}
	if byID[queued].BatchID != batch {
		t.Fatalf("batch %+v", byID[queued])
	}
	all, err := s.Operations(ctx, "", 10)
	if err != nil || len(all) < 3 {
		t.Fatalf("all: %d %v", len(all), err)
	}
	if _, err := s.Operations(ctx, "not-a-uuid", 10); err == nil {
		t.Fatal("a bad id must not query")
	}
}

func TestPowerReviewIsIdempotentHashBoundOneShotAndRecoversUnknown(t *testing.T) {
	s, ctx, tenant := testStore(t)
	host, err := s.Create(ctx, tenant, "power-review-host", "Power review", "lab", "operator")
	if err != nil {
		t.Fatal(err)
	}
	const endpointSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const actionSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const ticketSHA = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	key := uuid.NewString()
	id := uuid.NewString()
	input := PowerReview{ID: id, ServerID: host.ID, Guest: "guest-a", Verb: "reboot", Provider: "incus", EndpointSHA256: endpointSHA,
		ActionSHA256: actionSHA, IdempotencyKey: key, AgentHubTicket: "shp-" + strings.ReplaceAll(id, "-", ""), Actor: "operator", ActorName: "Operator", Note: "Restart after maintenance", ReviewWindowMinutes: 60}
	created, fresh, err := s.ReservePowerReview(ctx, input)
	if err != nil || !fresh || created.State != "preparing" {
		t.Fatalf("reserve: %+v fresh=%v err=%v", created, fresh, err)
	}
	replay := input
	replay.ID = uuid.NewString()
	replay.AgentHubTicket = "shp-" + strings.ReplaceAll(replay.ID, "-", "")
	got, fresh, err := s.ReservePowerReview(ctx, replay)
	if err != nil || fresh || got.ID != created.ID {
		t.Fatalf("idempotent replay: %+v fresh=%v err=%v", got, fresh, err)
	}
	conflict := replay
	conflict.ActionSHA256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	if _, _, err := s.ReservePowerReview(ctx, conflict); !errors.Is(err, ErrPowerReviewConflict) {
		t.Fatalf("changed action reused key: %v", err)
	}
	if _, err := s.SetPowerReviewState(ctx, created.ID, "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	provenance := map[string]any{"decision_id": "decision-test", "action_sha256": actionSHA}
	claimed, err := s.ClaimPowerReview(ctx, created.ID, host.ID, actionSHA, ticketSHA, provenance)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	claimed, err = s.ClaimPowerReview(ctx, created.ID, host.ID, actionSHA, ticketSHA, provenance)
	if err != nil || claimed {
		t.Fatalf("approval replay was claimed: claimed=%v err=%v", claimed, err)
	}
	if err := s.FinishPowerReview(ctx, created.ID, "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	final, err := s.GetPowerReview(ctx, created.ID, host.ID)
	if err != nil || final.State != "succeeded" || len(final.Provenance) == 0 {
		t.Fatalf("final review: %+v err=%v", final, err)
	}

	other := input
	other.ID = uuid.NewString()
	other.IdempotencyKey = uuid.NewString()
	other.AgentHubTicket = "shp-" + strings.ReplaceAll(other.ID, "-", "")
	interrupted, _, err := s.ReservePowerReview(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPowerReviewState(ctx, interrupted.ID, "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.ClaimPowerReview(ctx, interrupted.ID, host.ID, actionSHA, ticketSHA, provenance); err != nil || !claimed {
		t.Fatalf("claim interrupted action: %v %v", claimed, err)
	}
	if n, err := s.RecoverPowerReviews(ctx); err != nil || n != 1 {
		t.Fatalf("recover interrupted action: n=%d err=%v", n, err)
	}
	recovered, err := s.GetPowerReview(ctx, interrupted.ID, host.ID)
	if err != nil || recovered.State != "unknown" {
		t.Fatalf("recovered state: %+v err=%v", recovered, err)
	}
	var events int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_infra_power_review_events WHERE review_id=$1`, created.ID).Scan(&events); err != nil || events != 4 {
		t.Fatalf("review transition journal count=%d err=%v", events, err)
	}
	expiring := input
	expiring.ID = uuid.NewString()
	expiring.IdempotencyKey = uuid.NewString()
	expiring.AgentHubTicket = "shp-" + strings.ReplaceAll(expiring.ID, "-", "")
	expiring.ReviewWindowMinutes = 60
	expiringReview, _, err := s.ReservePowerReview(ctx, expiring)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPowerReviewState(ctx, expiringReview.ID, "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE shippin_infra_power_reviews SET expires_at=now()-interval '1 second' WHERE id=$1`, expiringReview.ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.ClaimPowerReview(ctx, expiringReview.ID, host.ID, actionSHA, ticketSHA, provenance); err != nil || claimed {
		t.Fatalf("expired review must not be claimed: claimed=%v err=%v", claimed, err)
	}
	expired, err := s.GetPowerReview(ctx, expiringReview.ID, host.ID)
	if err != nil || expired.State != "expired" || expired.FailureCode != "review_expired" {
		t.Fatalf("expired review state: %+v err=%v", expired, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_infra_power_review_events WHERE review_id=$1`, expiringReview.ID).Scan(&events); err != nil || events != 3 {
		t.Fatalf("expiry event journal count=%d err=%v", events, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE shippin_infra_power_review_events SET state='failed' WHERE review_id=$1`, created.ID); err == nil {
		t.Fatal("append-only review events accepted an update")
	}
}

func TestSecretsAndFleetsSurviveTheDatabase(t *testing.T) {
	s, ctx, _ := testStore(t)
	sealer, err := NewSealer(testKey)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	box, _ := sealer.Seal([]byte("token"), id)
	e, err := s.CreateEndpoint(ctx, Endpoint{ID: id, Slug: "sealed-host", Label: "Sealed", Site: "lab", Kind: "proxmox", BaseURL: "https://x.invalid", Node: "n", TokenID: "t", SealedSecret: box, TrustedCert: "pem"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if !e.HasSecret || e.TrustedCert != "pem" {
		t.Fatalf("%+v", e)
	}
	plain, err := sealer.Open(e.SealedSecret, id)
	if err != nil || string(plain) != "token" {
		t.Fatalf("%q %v", plain, err)
	}
	if _, err := s.CreateEndpoint(ctx, Endpoint{ID: uuid.NewString(), Slug: "sealed-host", Label: "Dup", Site: "lab", Kind: "proxmox"}, "u"); err != ErrSlugTaken {
		t.Fatalf("duplicate slug: %v", err)
	}
	f, err := s.SaveFleet(ctx, Fleet{Label: "Production", Description: "Both"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetFleet(ctx, id, f.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Endpoint(ctx, id)
	if got.FleetID != f.ID {
		t.Fatalf("fleet %q", got.FleetID)
	}
	if err := s.SetFleet(ctx, id, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatal("a fleet that does not exist was accepted")
	}
	// Detaching clears the API and the fleet, and keeps the row.
	if err := s.SetEndpoint(ctx, Endpoint{ID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Endpoint(ctx, id); err != ErrNotFound {
		t.Fatalf("detached: %v", err)
	}
	refs, _ := s.Registry(ctx)
	found := false
	for _, r := range refs {
		found = found || r.ID == id
	}
	if !found {
		t.Fatal("the row should stay in the registry")
	}
	if err := s.DeleteFleet(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFleet(ctx, f.ID); err != ErrNotFound {
		t.Fatalf("second delete: %v", err)
	}
}

func TestRecentOutcomesAreTheLatestSuccessPerGuest(t *testing.T) {
	s, ctx, tenant := testStore(t)
	g, err := s.Create(ctx, tenant, "recent-host", "Recent", "lab", "u")
	if err != nil {
		t.Fatal(err)
	}
	finish := func(guest, status, guestStatus string) {
		t.Helper()
		id, err := s.StartOperation(ctx, Operation{ServerID: g.ID, Guest: guest, Verb: "stop", Actor: "u"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.FinishOperation(ctx, id, status, "", guestStatus); err != nil {
			t.Fatal(err)
		}
	}
	finish("a", OpSucceeded, "running")
	finish("a", OpSucceeded, "stopped")
	finish("b", OpFailed, "running")
	got, err := s.RecentOutcomes(ctx, g.ID, time.Minute)
	if err != nil || len(got) != 1 || got["a"] != "stopped" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE infra_operations SET finished_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	got, err = s.RecentOutcomes(ctx, g.ID, time.Minute)
	if err != nil || len(got) != 0 {
		t.Fatalf("an old result must not outrank the inventory: %v %v", got, err)
	}
}
