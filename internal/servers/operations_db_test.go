package servers

import (
	"errors"
	"strings"
	"testing"

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
