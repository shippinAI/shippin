// Package goldverify is the Gold-side passkey verifier. It is stdlib-only so it can be vendored
// into gold-policy as a subcommand. It holds no private key: it checks a WebAuthn assertion made by
// the operator's passkey against a pinned public key, offline, and only then writes the approval file.
package goldverify

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Payload types (signed, so a signature for one kind can never be read as another).
const (
	TypeApproval = "approval" // decision approve|deny on one ticket sha256
	TypeRevoke   = "revoke"   // decision revoke of one bypass cert
)

// Payload is what the operator signs. Ticket decisions use TicketID+TicketSHA256; revocations use CID.
type Payload struct {
	Type         string `json:"type"` // approval | revoke; must match Decision
	TicketID     string `json:"ticket_id,omitempty"`
	TicketSHA256 string `json:"ticket_sha256,omitempty"`
	CID          string `json:"cid,omitempty"`
	Decision     string `json:"decision"` // approve | deny | revoke
	TS           int64  `json:"ts"`       // unix seconds, set by the signing device
}

var (
	reID  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	reSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Validate rejects anything that is not exactly one well-formed decision.
func (p Payload) Validate() error {
	switch p.Decision {
	case "approve", "deny":
		if p.Type != TypeApproval {
			return errors.New("type must be approval")
		}
		if !reID.MatchString(p.TicketID) || !reSHA.MatchString(p.TicketSHA256) || p.CID != "" {
			return errors.New("bad ticket payload")
		}
	case "revoke":
		if p.Type != TypeRevoke {
			return errors.New("type must be revoke")
		}
		if !reID.MatchString(p.CID) || p.TicketID != "" || p.TicketSHA256 != "" {
			return errors.New("bad revoke payload")
		}
	default:
		return fmt.Errorf("bad decision %q", p.Decision)
	}
	if p.TS <= 0 {
		return errors.New("bad ts")
	}
	return nil
}

// Canonical is the JCS form (keys sorted, no spaces). Validate guarantees no value needs escaping,
// so this string is byte-identical to what the console builds in canonical.ts.
func (p Payload) Canonical() string {
	ts := strconv.FormatInt(p.TS, 10)
	if p.Decision == "revoke" {
		return `{"cid":"` + p.CID + `","decision":"revoke","ts":` + ts + `,"type":"revoke"}`
	}
	return `{"decision":"` + p.Decision + `","ticket_id":"` + p.TicketID +
		`","ticket_sha256":"` + p.TicketSHA256 + `","ts":` + ts + `,"type":"approval"}`
}

// Challenge is sha256(canonical payload); the WebAuthn challenge bytes.
func (p Payload) Challenge() []byte {
	h := sha256.Sum256([]byte(p.Canonical()))
	return h[:]
}

// B64 is unpadded base64url, the encoding WebAuthn uses in clientDataJSON.
var B64 = base64.RawURLEncoding
