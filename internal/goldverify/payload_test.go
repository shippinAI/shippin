package goldverify

import (
	"strings"
	"testing"
)

// A signed payload's type must match its decision, so one kind can never be replayed as another.
func TestTypeMustMatchDecision(t *testing.T) {
	sha := strings.Repeat("a", 64)
	for _, p := range []Payload{
		{TicketID: "GC-1", TicketSHA256: sha, Decision: "approve", TS: 1},
		{Type: TypeRevoke, TicketID: "GC-1", TicketSHA256: sha, Decision: "approve", TS: 1},
		{Type: TypeApproval, CID: "cert-7", Decision: "revoke", TS: 1},
	} {
		if p.Validate() == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
}
