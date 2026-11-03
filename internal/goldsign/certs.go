package goldsign

import (
	"os"
	"path/filepath"
	"sort"
)

// Cert is a bypass (delegation) certificate as the cert service exports it (AGENT-TICKETS-PLAN
// §C). Read-only here; the console cannot issue one.
type Cert struct {
	CID         string   `json:"cid"`
	HolderAgent string   `json:"holder_agent"`
	Scope       []string `json:"scope"`
	NBF         int64    `json:"nbf"`
	Exp         int64    `json:"exp"`
	MaxUses     int      `json:"max_uses"`
	Uses        int      `json:"uses"`
	Reason      string   `json:"reason"`
	RevokeSent  bool     `json:"revoke_sent"`
}

// BypassStatus is what the header badge shows.
type BypassStatus struct {
	Active []Cert `json:"active"`
	Now    int64  `json:"now"`
	// Known is false when Gold has not synced recently or could not read its cert store: the badge
	// must then say the status is unknown, never "No bypass active".
	Known  bool  `json:"known"`
	SyncAt int64 `json:"sync_at"`
}

// Bypass lists certificates that are inside their window and not revoked on Gold (revoked/<cid>
// is synced back from Gold). A revoke that is only relayed still counts as active.
func (s *Store) Bypass() BypassStatus {
	now := s.now().Unix()
	files, _ := filepath.Glob(filepath.Join(s.Dir, "certs", "*.json"))
	last, fresh := s.LastSync()
	st := BypassStatus{Active: []Cert{}, Now: now, Known: fresh && last.CertsOK, SyncAt: last.At}
	for _, f := range files {
		var c Cert
		if readJSON(f, &c) != nil || !reID.MatchString(c.CID) {
			continue
		}
		if now < c.NBF || now >= c.Exp || exists(filepath.Join(s.Dir, "revoked", c.CID)) {
			continue
		}
		c.RevokeSent = exists(filepath.Join(s.Dir, "outbox", c.CID+"-revoke.json"))
		st.Active = append(st.Active, c)
	}
	sort.Slice(st.Active, func(i, j int) bool { return st.Active[i].Exp < st.Active[j].Exp })
	return st
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
