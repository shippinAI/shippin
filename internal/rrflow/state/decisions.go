package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// genesis is the prev hash of the first decision of an instance.
const genesis = "0000000000000000000000000000000000000000000000000000000000000000"

// hashOf chains d to its predecessor: sha256(prev || json(d without hash)).
func hashOf(d Decision) string {
	d.Hash = ""
	b, _ := json.Marshal(d)
	h := sha256.Sum256(append([]byte(d.Prev), b...))
	return hex.EncodeToString(h[:])
}

// AppendDecision writes one decision row; it is never updated or deleted afterwards.
func (s *Store) AppendDecision(d Decision) (*Decision, error) {
	if !validWord(d.Action) || !effects[d.Effect] || d.Actor == "" || len(d.Actor) > 128 ||
		len(d.Resource) > maxText || len(d.Reason) > maxText || len(d.ConfigVer) > 64 {
		return nil, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.Session != "" {
		if x, ok := s.sessions[d.Session]; !ok || x.Workspace != d.Workspace {
			return nil, ErrNotFound
		}
	}
	id, _ := uuid.NewV7()
	d.ID, d.OrgID, d.At, d.Prev = id.String(), s.org, time.Now().UTC(), genesis
	d.Seq = int64(len(s.decisions)) + 1
	if n := len(s.decisions); n > 0 {
		d.Prev = s.decisions[n-1].Hash
	}
	d.Hash = hashOf(d)
	if err := s.appendLine(decisionsFile, d); err != nil {
		return nil, err
	}
	s.decisions = append(s.decisions, d)
	return &d, nil
}

// Decisions lists the newest decisions first, filtered to workspace unless it is empty.
func (s *Store) Decisions(workspace string, limit int) []Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Decision{}
	for i := len(s.decisions) - 1; i >= 0 && len(out) < limit; i-- {
		if workspace == "" || s.decisions[i].Workspace == workspace {
			out = append(out, s.decisions[i])
		}
	}
	return out
}

// Verify re-walks the chain; it reports the first row whose link or hash is wrong.
func (s *Store) Verify() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := genesis
	for i, d := range s.decisions {
		if d.Prev != prev || d.Seq != int64(i)+1 || hashOf(d) != d.Hash {
			return i, fmt.Errorf("decision chain broken at seq %d", d.Seq)
		}
		prev = d.Hash
	}
	return len(s.decisions), nil
}
