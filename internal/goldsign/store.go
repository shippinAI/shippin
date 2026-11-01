// Package goldsign is the console side of passkey signing for Gold approvals. It shows facts,
// relays WebAuthn envelopes and stores public keys. It holds no signing key and never writes an
// approval: only gold-passkey-verify on Gold does, after verifying against its own pinned keys.
package goldsign

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"shippin.cloud/console/internal/goldverify"
)

// Facts is the gold-policy review export for one ticket: parsed by gold-policy on Gold, synced
// read-only into FactsDir. AgentSummary is shown separately and labelled as agent text.
type Facts struct {
	ID           string   `json:"id"`
	SHA256       string   `json:"sha256"`
	Tier         string   `json:"tier"`
	TierReasons  []string `json:"tier_reasons"`
	Classes      []string `json:"classes"`
	Host         string   `json:"host"`
	OOBPath      string   `json:"oob_path"`
	Commands     []string `json:"commands"`
	Rollback     []string `json:"rollback"`
	Verification []string `json:"verification"`
	RequestedAt  int64    `json:"requested_at"`
	AgentSummary string   `json:"agent_summary"`
	Sent         string   `json:"sent,omitempty"` // decision relayed to Gold from this console
}

// Store is file-backed: FactsDir is read-only input, Dir holds keys/, outbox/, certs/, revoked/.
type Store struct {
	Dir, FactsDir string
	OwnerSub      string // the operator's Nomen subject; the only caller allowed to register or sign
	OwnerName     string // approver name pinned keys carry (Gold's GOLD_APPROVER)
	RPID          string
	Origins       []string
	Now           func() time.Time
}

var reID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ErrNotFound is returned for unknown tickets/certs.
var ErrNotFound = errors.New("not found")

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// IsOwner is the single gate for every write.
func (s *Store) IsOwner(sub string) bool { return s.OwnerSub != "" && sub == s.OwnerSub }

// Pending lists tickets awaiting a decision, newest first.
func (s *Store) Pending() ([]Facts, error) {
	files, err := filepath.Glob(filepath.Join(s.FactsDir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := []Facts{}
	for _, f := range files {
		var x Facts
		if readJSON(f, &x) == nil && reID.MatchString(x.ID) {
			x.Sent = s.sent(x.ID, x.SHA256)
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt > out[j].RequestedAt })
	return out, nil
}

// Ticket returns one ticket's facts.
func (s *Store) Ticket(id string) (*Facts, error) {
	if !reID.MatchString(id) {
		return nil, ErrNotFound
	}
	var x Facts
	if err := readJSON(filepath.Join(s.FactsDir, id+".json"), &x); err != nil || x.ID != id {
		return nil, ErrNotFound
	}
	x.Sent = s.sent(id, x.SHA256)
	return &x, nil
}

// Relay queues an envelope for Gold. Shape and binding are checked so typos fail fast, but the
// console's opinion carries no authority: Gold re-verifies everything.
func (s *Store) Relay(e *goldverify.Envelope) error {
	p := e.Payload
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Decision == "revoke" && !exists(filepath.Join(s.Dir, "certs", p.CID+".json")) {
		return ErrNotFound
	}
	if p.Decision != "revoke" {
		t, err := s.Ticket(p.TicketID)
		if err != nil || t.SHA256 != p.TicketSHA256 {
			return errors.New("envelope does not match the ticket's current sha256")
		}
	}
	name := p.TicketID + p.CID
	return writeJSON(filepath.Join(s.Dir, "outbox", name+"-"+p.Decision+".json"), e)
}

// Outbox returns queued envelopes for Gold's fetch unit.
func (s *Store) Outbox() ([]json.RawMessage, error) {
	files, _ := filepath.Glob(filepath.Join(s.Dir, "outbox", "*.json"))
	out := []json.RawMessage{}
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil {
			out = append(out, b)
		}
	}
	return out, nil
}

func (s *Store) sent(id, sha string) string {
	for _, d := range []string{"approve", "deny"} {
		var e goldverify.Envelope
		if readJSON(filepath.Join(s.Dir, "outbox", id+"-"+d+".json"), &e) == nil && e.Payload.TicketSHA256 == sha {
			return d
		}
	}
	return ""
}
