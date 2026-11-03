package goldsign

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"shippin.cloud/console/internal/goldverify"
)

// Sync is one full snapshot pushed by Gold's gold-sign-export (root unit, over the tailnet): the
// gold-policy review facts of every undecided ticket, Gold's accepted bypass certs, the revoked cert
// ids and the challenges Gold has consumed. It carries no key and cannot create an approval.
type Sync struct {
	Facts   []Facts  `json:"facts"`
	Certs   []Cert   `json:"certs"`
	CertsOK bool     `json:"certs_ok"` // false: Gold could not read its cert store (badge says unknown)
	Revoked []string `json:"revoked"`
	Used    []string `json:"used"` // hex sha256 challenges already verified on Gold
	At      int64    `json:"at"`
}

// SyncState is the time of the last applied sync; the badge goes "unknown" when it is stale.
type SyncState struct {
	At      int64 `json:"at"`
	CertsOK bool  `json:"certs_ok"`
}

// StaleAfter: no sync for this long means the console no longer knows Gold's state.
const StaleAfter = 2 * time.Minute

var reHex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (x Facts) valid() bool { return reID.MatchString(x.ID) && reHex64.MatchString(x.SHA256) }

// ApplySync replaces facts/, certs/ and revoked/ with Gold's snapshot and drops consumed outbox
// envelopes. Invalid entries reject the whole push (nothing half-applied).
func (s *Store) ApplySync(in *Sync) error {
	for _, f := range in.Facts {
		if !f.valid() {
			return fmt.Errorf("bad facts entry %q", f.ID)
		}
	}
	for _, c := range in.Certs {
		if !reID.MatchString(c.CID) || c.HolderAgent == "" || c.Exp <= c.NBF {
			return fmt.Errorf("bad cert %q", c.CID)
		}
	}
	for _, r := range in.Revoked {
		if !reID.MatchString(r) {
			return errors.New("bad revoked id")
		}
	}
	files := map[string]map[string]any{"facts": {}, "certs": {}, "revoked": {}}
	for _, f := range in.Facts {
		f.Sent = ""
		files["facts"][f.ID+".json"] = f
	}
	for _, c := range in.Certs {
		c.RevokeSent = false
		files["certs"][c.CID+".json"] = c
	}
	for _, r := range in.Revoked {
		files["revoked"][r] = map[string]string{"cid": r}
	}
	for dir, m := range files {
		if err := s.replaceDir(dir, m); err != nil {
			return err
		}
	}
	s.dropUsed(in.Used)
	return writeJSON(filepath.Join(s.Dir, "sync.json"), SyncState{At: s.now().Unix(), CertsOK: in.CertsOK})
}

// replaceDir writes a new dir then swaps it in, so readers never see a half-written set.
func (s *Store) replaceDir(name string, files map[string]any) error {
	dst := s.dirFor(name)
	tmp := dst + ".new"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o750); err != nil {
		return err
	}
	for fn, v := range files {
		if err := writeJSON(filepath.Join(tmp, fn), v); err != nil {
			return err
		}
	}
	old := dst + ".old"
	os.RemoveAll(old)
	if err := os.Rename(dst, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return os.RemoveAll(old)
}

func (s *Store) dirFor(name string) string {
	if name == "facts" {
		return s.FactsDir
	}
	return filepath.Join(s.Dir, name)
}

func (s *Store) dropUsed(used []string) {
	set := map[string]bool{}
	for _, u := range used {
		set[u] = true
	}
	files, _ := filepath.Glob(filepath.Join(s.Dir, "outbox", "*.json"))
	for _, f := range files {
		var e goldverify.Envelope
		if readJSON(f, &e) == nil && e.Payload.Validate() == nil && set[fmt.Sprintf("%x", e.Payload.Challenge())] {
			os.Remove(f)
		}
	}
}

// LastSync returns the last applied sync and whether it is still fresh.
func (s *Store) LastSync() (SyncState, bool) {
	var st SyncState
	if readJSON(filepath.Join(s.Dir, "sync.json"), &st) != nil {
		return st, false
	}
	return st, s.now().Sub(time.Unix(st.At, 0)) <= StaleAfter
}
