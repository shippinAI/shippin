package goldverify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Config is everything the verifier needs; all paths are Gold paths set by the root unit.
type Config struct {
	Approver    string // OS user that owns approvals (operator)
	ApproverUID int    // -1 in tests: skip chown
	ApproverGID int
	Keys        []PinnedKey   // pinned public keys
	TicketDir   string        // /var/lib/shippin/changes/tickets
	ApprovalDir string        // /var/lib/shippin/changes/approvals
	StateDir    string        // /var/lib/gold-passkey (root only): used-set, revocations
	MaxSkew     time.Duration // how old an assertion may be (default 5m)
	Now         func() time.Time
}

// Result says what was written; Reason is set when nothing was.
type Result struct {
	OK     bool
	Wrote  string
	Reason string
}

func (c *Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Verify checks one envelope end to end and, only if every check passes, writes the approval
// (or revocation) and a decision-log entry. Every outcome is logged.
func (c *Config) Verify(e *Envelope) Result {
	r := c.verify(e)
	c.logDecision(e, r)
	return r
}

func (c *Config) verify(e *Envelope) Result {
	fail := func(f string, a ...any) Result { return Result{Reason: fmt.Sprintf(f, a...)} }
	p := e.Payload
	if err := p.Validate(); err != nil {
		return fail("%v", err)
	}
	k, err := find(c.Keys, e.KID, c.Approver)
	if err != nil {
		return fail("%v", err)
	}
	if err := checkAssertion(e, k); err != nil {
		return fail("%v", err)
	}
	maxSkew := c.MaxSkew
	if maxSkew == 0 {
		maxSkew = 5 * time.Minute
	}
	now := c.now()
	age := now.Sub(time.Unix(p.TS, 0))
	if age > maxSkew {
		return fail("assertion expired (%s old)", age.Truncate(time.Second))
	}
	if age < -time.Minute {
		return fail("assertion ts is in the future")
	}
	if p.Decision != "revoke" {
		got, err := fileSHA(filepath.Join(c.TicketDir, p.TicketID+".md"))
		if err != nil {
			return fail("ticket unreadable: %v", err)
		}
		if got != p.TicketSHA256 {
			return fail("ticket sha256 mismatch (ticket changed or wrong ticket)")
		}
	}
	// Replay: last check before writing, so a failed write cannot burn a valid tap silently.
	if err := c.markUsed(p.Challenge()); err != nil {
		return fail("%v", err)
	}
	if p.Decision == "revoke" {
		path, err := c.writeRevocation(p, k.KID, now)
		if err != nil {
			return fail("revocation write failed: %v", err)
		}
		return Result{OK: true, Wrote: path}
	}
	path, err := c.writeApproval(p, k.KID, now)
	if err != nil {
		return fail("approval write failed: %v", err)
	}
	return Result{OK: true, Wrote: path}
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Seen reports whether this envelope's challenge was already used, so `drain` can skip envelopes
// the console outbox still lists without logging a refusal every poll.
func (c *Config) Seen(e *Envelope) bool {
	if e.Payload.Validate() != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(c.StateDir, "used", hex.EncodeToString(e.Payload.Challenge())))
	return err == nil
}

// markUsed records the challenge in the root-owned used-set; O_EXCL makes a replay fail.
func (c *Config) markUsed(ch []byte) error {
	dir := filepath.Join(c.StateDir, "used")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, hex.EncodeToString(ch)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		return fmt.Errorf("replay: this assertion was already used")
	}
	if err != nil {
		return err
	}
	return f.Close()
}
