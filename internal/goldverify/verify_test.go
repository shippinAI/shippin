package goldverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApproveWritesGoldApproveFormat(t *testing.T) {
	j := newSigner(t, "operator-phone", "operator")
	c, sha := setup(t, j.pin)
	r := c.Verify(j.sign(t, approvePayload(sha), flagUP|flagUV, testOrigin))
	if !r.OK {
		t.Fatal(r.Reason)
	}
	b, _ := os.ReadFile(r.Wrote)
	want := "decision: approved\napproved_by: operator\nidentity: operator@passkey:operator-phone\nticket_id: GC-T1\n" +
		"ticket_sha256: " + sha + "\napproved_at: 1790000000\nreason: passkey approval (operator-phone)\n"
	if string(b) != want {
		t.Fatalf("got\n%s", b)
	}
	st, _ := os.Stat(r.Wrote)
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", st.Mode())
	}
	log, _ := os.ReadFile(filepath.Join(c.ApprovalDir, "decisions.jsonl"))
	if !strings.Contains(string(log), `"source":"gold-passkey","event":"approve","id":"GC-T1","result":"approved"`) {
		t.Fatalf("log: %s", log)
	}
}

func TestDenyWritesRejected(t *testing.T) {
	j := newSigner(t, "k1", "operator")
	c, sha := setup(t, j.pin)
	p := approvePayload(sha)
	p.Decision = "deny"
	r := c.Verify(j.sign(t, p, flagUP|flagUV, testOrigin))
	b, _ := os.ReadFile(r.Wrote)
	if !r.OK || !strings.HasPrefix(string(b), "decision: rejected\n") {
		t.Fatal(r.Reason, string(b))
	}
}

func TestRefusals(t *testing.T) {
	j := newSigner(t, "k1", "operator")
	other := newSigner(t, "agent-key", "node")
	unpinned := newSigner(t, "k9", "operator")
	cases := []struct {
		name, want string
		env        func(sha string) *Envelope
	}{
		{"wrong hash", "sha256 mismatch", func(sha string) *Envelope {
			p := approvePayload(strings.Repeat("0", 64))
			return j.sign(t, p, flagUP|flagUV, testOrigin)
		}},
		{"expired", "expired", func(sha string) *Envelope {
			p := approvePayload(sha)
			p.TS = t0.Add(-6 * time.Minute).Unix()
			return j.sign(t, p, flagUP|flagUV, testOrigin)
		}},
		{"future", "future", func(sha string) *Envelope {
			p := approvePayload(sha)
			p.TS = t0.Add(5 * time.Minute).Unix()
			return j.sign(t, p, flagUP|flagUV, testOrigin)
		}},
		{"non-operator key", "belongs to \"node\"", func(sha string) *Envelope { return other.sign(t, approvePayload(sha), flagUP|flagUV, testOrigin) }},
		{"unpinned key", "not pinned", func(sha string) *Envelope { return unpinned.sign(t, approvePayload(sha), flagUP|flagUV, testOrigin) }},
		{"forged with other key under operator kid", "bad signature", func(sha string) *Envelope {
			e := unpinned.sign(t, approvePayload(sha), flagUP|flagUV, testOrigin)
			e.KID, e.CredentialID = j.kid, j.cred
			return e
		}},
		{"payload edited after signing", "challenge does not match", func(sha string) *Envelope {
			e := j.sign(t, approvePayload(sha), flagUP|flagUV, testOrigin)
			e.Payload.Decision = "deny"
			return e
		}},
		{"no user verification", "verification required", func(sha string) *Envelope { return j.sign(t, approvePayload(sha), flagUP, testOrigin) }},
		{"wrong origin", "origin not allowed", func(sha string) *Envelope {
			return j.sign(t, approvePayload(sha), flagUP|flagUV, "https://evil.invalid")
		}},
		{"bad ticket id", "bad ticket payload", func(sha string) *Envelope {
			p := approvePayload(sha)
			p.TicketID = "../etc/x"
			return j.sign(t, p, flagUP|flagUV, testOrigin)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, sha := setup(t, j.pin, other.pin)
			r := c.Verify(tc.env(sha))
			if r.OK || !strings.Contains(r.Reason, tc.want) {
				t.Fatalf("want refusal %q, got ok=%v %q", tc.want, r.OK, r.Reason)
			}
			noApproval(t, c)
			log, _ := os.ReadFile(filepath.Join(c.ApprovalDir, "decisions.jsonl"))
			if !strings.Contains(string(log), `"result":"unverified-passkey"`) {
				t.Fatalf("refusal not logged: %s", log)
			}
		})
	}
}

func TestReplayRefused(t *testing.T) {
	j := newSigner(t, "k1", "operator")
	c, sha := setup(t, j.pin)
	e := j.sign(t, approvePayload(sha), flagUP|flagUV, testOrigin)
	if r := c.Verify(e); !r.OK {
		t.Fatal(r.Reason)
	}
	if !c.Seen(e) {
		t.Fatal("Seen must report a used envelope")
	}
	os.Remove(filepath.Join(c.ApprovalDir, "GC-T1.approval"))
	if r := c.Verify(e); r.OK || !strings.Contains(r.Reason, "replay") {
		t.Fatalf("replay accepted: %+v", r)
	}
	noApproval(t, c)
}

func TestRevoke(t *testing.T) {
	j := newSigner(t, "k1", "operator")
	c, _ := setup(t, j.pin)
	r := c.Verify(j.sign(t, Payload{Type: TypeRevoke, CID: "cert-7", Decision: "revoke", TS: t0.Unix()}, flagUP|flagUV, testOrigin))
	if !r.OK || !strings.HasSuffix(r.Wrote, "revoked/cert-7") {
		t.Fatalf("%+v", r)
	}
}

func TestCanonicalVector(t *testing.T) {
	p := Payload{Type: TypeApproval, TicketID: "GC-1", TicketSHA256: strings.Repeat("a", 64), Decision: "approve", TS: 1}
	want := `{"decision":"approve","ticket_id":"GC-1","ticket_sha256":"` + strings.Repeat("a", 64) + `","ts":1,"type":"approval"}`
	if p.Canonical() != want {
		t.Fatal(p.Canonical())
	}
	// Same vector is asserted in web/src/lib/gold-sign/canonical.test.ts.
	if got := B64.EncodeToString(p.Challenge()); got != "6mjg305JKH7E9sjXHlhfX3KZpCU4YfhpsartOgCavYA" {
		t.Fatal("challenge vector:", got)
	}
}
