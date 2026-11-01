package goldexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Env holds the Gold paths (from /etc/shippin/signer/passkey.env).
type Env struct {
	PolicyBin, TicketDir, ApprovalDir, StateDir, CertDir string
	Now                                                  func() time.Time
}

// Sync is the console's goldsign.Sync wire format.
type Sync struct {
	Facts   []Facts  `json:"facts"`
	Certs   []Cert   `json:"certs"`
	CertsOK bool     `json:"certs_ok"`
	Revoked []string `json:"revoked"`
	Used    []string `json:"used"`
	At      int64    `json:"at"`
}

var reID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Collect builds one snapshot. Tickets that already have an approval file for their current sha256
// are decided and left out; a ticket whose review fails is left out and reported in errs.
func (e *Env) Collect(ctx context.Context) (*Sync, []error) {
	s := &Sync{Facts: []Facts{}, Certs: []Cert{}, Revoked: []string{}, Used: []string{}, At: e.Now().Unix()}
	var errs []error
	tickets, _ := filepath.Glob(filepath.Join(e.TicketDir, "*.md"))
	for _, t := range tickets {
		id := strings.TrimSuffix(filepath.Base(t), ".md")
		if !reID.MatchString(id) {
			continue
		}
		f, err := e.facts(ctx, id, t)
		if err != nil {
			errs = append(errs, err)
		} else if f != nil {
			s.Facts = append(s.Facts, *f)
		}
	}
	s.Certs, s.CertsOK = e.certs()
	s.Revoked = names(filepath.Join(e.StateDir, "revoked"), 0)
	s.Used = names(filepath.Join(e.StateDir, "used"), 48*time.Hour)
	return s, errs
}

func (e *Env) facts(ctx context.Context, id, path string) (*Facts, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	sha := hex.EncodeToString(sum[:])
	if a, err := os.ReadFile(filepath.Join(e.ApprovalDir, id+".approval")); err == nil && strings.Contains(string(a), "\nticket_sha256: "+sha+"\n") {
		return nil, nil // decided for this exact content
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, e.PolicyBin, "review", id).Output()
	if err != nil {
		return nil, fmt.Errorf("review %s: %v", id, err)
	}
	f, err := ParseReview(string(out))
	if err != nil {
		return nil, err
	}
	if f.ID != id || f.SHA256 != sha {
		return nil, fmt.Errorf("review %s: sha256 differs from the file (changed while reading)", id)
	}
	st, _ := os.Stat(path)
	f.RequestedAt = st.ModTime().Unix()
	f.AgentSummary = field(string(b), "title")
	return f, nil
}

// Cert is the badge's view of one accepted bypass certificate (AGENT-TICKETS-PLAN §C); extra
// fields in the store (envelope, holder key) are dropped here and never leave Gold.
type Cert struct {
	CID         string   `json:"cid"`
	HolderAgent string   `json:"holder_agent"`
	Scope       []string `json:"scope"`
	NBF         int64    `json:"nbf"`
	Exp         int64    `json:"exp"`
	MaxUses     int      `json:"max_uses"`
	Uses        int      `json:"uses"`
	Reason      string   `json:"reason"`
}

// certs reads Gold's accepted bypass-cert store; ok=false if it exists but cannot be read.
func (e *Env) certs() ([]Cert, bool) {
	out := []Cert{}
	if _, err := os.Stat(e.CertDir); os.IsNotExist(err) {
		return out, true // no cert source installed: honestly none
	}
	files, err := filepath.Glob(filepath.Join(e.CertDir, "*.json"))
	if err != nil {
		return out, false
	}
	for _, f := range files {
		var c Cert
		b, err := os.ReadFile(f)
		if err != nil || json.Unmarshal(b, &c) != nil || !reID.MatchString(c.CID) {
			return out, false
		}
		out = append(out, c)
	}
	return out, true
}

func names(dir string, maxAge time.Duration) []string {
	out := []string{}
	ents, _ := os.ReadDir(dir)
	for _, d := range ents {
		info, err := d.Info()
		if err != nil || strings.HasSuffix(d.Name(), ".tmp") || (maxAge > 0 && time.Since(info.ModTime()) > maxAge) {
			continue
		}
		out = append(out, d.Name())
	}
	return out
}

func field(doc, k string) string {
	for _, l := range strings.Split(doc, "\n") {
		if v, ok := strings.CutPrefix(l, k+":"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
