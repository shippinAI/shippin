package goldsign

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/goldverify"
)

func TestSyncReplacesFactsAndDropsUsedEnvelopes(t *testing.T) {
	s := newStore(t)
	a64 := strings.Repeat("a", 64)
	e := &goldverify.Envelope{Payload: goldverify.Payload{Type: goldverify.TypeApproval, TicketID: "GC-1", TicketSHA256: a64, Decision: "approve", TS: 1}}
	if err := s.Relay(e); err != nil {
		t.Fatal(err)
	}
	in := &Sync{CertsOK: true, Facts: []Facts{{ID: "GC-2", SHA256: strings.Repeat("b", 64), Tier: "T2"}},
		Used: []string{fmt.Sprintf("%x", e.Payload.Challenge())}}
	if err := s.ApplySync(in); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Pending()
	if len(p) != 1 || p[0].ID != "GC-2" {
		t.Fatalf("facts not replaced: %+v", p)
	}
	if out, _ := s.Outbox(); len(out) != 0 {
		t.Fatal("used envelope not dropped from outbox")
	}
}

func TestSyncRejectsBadEntriesWhole(t *testing.T) {
	s := newStore(t)
	bad := []*Sync{
		{Facts: []Facts{{ID: "../x", SHA256: strings.Repeat("a", 64)}}},
		{Facts: []Facts{{ID: "GC-9", SHA256: "short"}}},
		{Certs: []Cert{{CID: "c1", HolderAgent: "node", NBF: 10, Exp: 5}}},
		{Revoked: []string{"a/b"}},
	}
	for i, in := range bad {
		if s.ApplySync(in) == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	if p, _ := s.Pending(); len(p) != 1 || p[0].ID != "GC-1" {
		t.Fatal("a rejected push changed facts")
	}
}

func TestBypassKnownOnlyWhenFresh(t *testing.T) {
	s := newStore(t)
	if s.Bypass().Known {
		t.Fatal("never synced must be unknown")
	}
	n := now.Unix()
	in := &Sync{CertsOK: true, Certs: []Cert{{CID: "c1", HolderAgent: "node", Scope: []string{"gold:T1"}, NBF: n - 1, Exp: n + 600}}, Revoked: []string{"c0"}}
	if err := s.ApplySync(in); err != nil {
		t.Fatal(err)
	}
	if st := s.Bypass(); !st.Known || len(st.Active) != 1 {
		t.Fatalf("%+v", st)
	}
	if !exists(filepath.Join(s.Dir, "revoked", "c0")) {
		t.Fatal("revoked not synced")
	}
	s.Now = func() time.Time { return now.Add(StaleAfter + time.Second) }
	if s.Bypass().Known {
		t.Fatal("stale sync must be unknown")
	}
	s.Now = func() time.Time { return now }
	s.ApplySync(&Sync{CertsOK: false})
	if s.Bypass().Known {
		t.Fatal("cert store unreadable on Gold must be unknown")
	}
	os.RemoveAll(filepath.Join(s.Dir, "certs"))
}
