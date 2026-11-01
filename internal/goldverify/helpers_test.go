package goldverify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	testRP     = "console.example.invalid"
	testOrigin = "https://console.example.invalid"
	ticketBody = "id: GC-T1\ncommands:\n  - true\n"
)

var t0 = time.Unix(1_790_000_000, 0)

// signer is a fake authenticator: it holds a private key only inside the test process.
type signer struct {
	kid  string
	cred string
	priv *ecdsa.PrivateKey
	pin  PinnedKey
}

func newSigner(t *testing.T, kid, owner string) *signer {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	pin := PinnedKey{KID: kid, Owner: owner, CredentialID: "cred-" + kid, SPKI: B64.EncodeToString(der),
		Alg: -7, RPID: testRP, Origins: []string{testOrigin}}
	if err := pin.parse(); err != nil {
		t.Fatal(err)
	}
	return &signer{kid: kid, cred: pin.CredentialID, priv: priv, pin: pin}
}

// sign makes a WebAuthn assertion over p. flags defaults to UP|UV.
func (s *signer) sign(t *testing.T, p Payload, flags byte, origin string) *Envelope {
	t.Helper()
	rp := sha256.Sum256([]byte(testRP))
	ad := append(rp[:], flags, 0, 0, 0, 0)
	cd, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": B64.EncodeToString(p.Challenge()), "origin": origin})
	ch := sha256.Sum256(cd)
	d := sha256.Sum256(append(append([]byte{}, ad...), ch[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, s.priv, d[:])
	if err != nil {
		t.Fatal(err)
	}
	return &Envelope{Payload: p, KID: s.kid, CredentialID: s.cred, AuthenticatorData: B64.EncodeToString(ad),
		ClientDataJSON: B64.EncodeToString(cd), Signature: B64.EncodeToString(sig)}
}

func setup(t *testing.T, keys ...PinnedKey) (*Config, string) {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"tickets", "approvals", "state"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "tickets", "GC-T1.md"), []byte(ticketBody), 0o644)
	sum := sha256.Sum256([]byte(ticketBody))
	c := &Config{Approver: "operator", ApproverUID: -1, Keys: keys, TicketDir: filepath.Join(dir, "tickets"),
		ApprovalDir: filepath.Join(dir, "approvals"), StateDir: filepath.Join(dir, "state"), Now: func() time.Time { return t0 }}
	return c, hex.EncodeToString(sum[:])
}

func approvePayload(sha string) Payload {
	return Payload{Type: TypeApproval, TicketID: "GC-T1", TicketSHA256: sha, Decision: "approve", TS: t0.Unix() - 10}
}

func noApproval(t *testing.T, c *Config) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(c.ApprovalDir, "GC-T1.approval")); !os.IsNotExist(err) {
		t.Fatal("approval file must not exist")
	}
}
