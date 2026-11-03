package goldsign

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/goldverify"
)

var now = time.Unix(1_790_000_000, 0)

func newStore(t *testing.T) *Store {
	dir := t.TempDir()
	s := &Store{Dir: dir, FactsDir: filepath.Join(dir, "facts"), OwnerSub: "nomen|operator", OwnerName: "operator", RPID: "console.example.invalid",
		Origins: []string{"https://console.example.invalid"}, Now: func() time.Time { return now }}
	writeJSON(filepath.Join(s.FactsDir, "GC-1.json"), Facts{ID: "GC-1", SHA256: strings.Repeat("a", 64), RequestedAt: 1})
	return s
}

func TestOwnerGate(t *testing.T) {
	s := newStore(t)
	if !s.IsOwner("nomen|operator") || s.IsOwner("nomen|node") || s.IsOwner("") {
		t.Fatal("owner gate wrong")
	}
	if (&Store{}).IsOwner("") {
		t.Fatal("empty owner must match nobody")
	}
}

func TestRelayChecksSHAAndNeverWritesApprovals(t *testing.T) {
	s := newStore(t)
	bad := &goldverify.Envelope{Payload: goldverify.Payload{Type: goldverify.TypeApproval, TicketID: "GC-1", TicketSHA256: strings.Repeat("b", 64), Decision: "approve", TS: 1}}
	if s.Relay(bad) == nil {
		t.Fatal("stale sha relayed")
	}
	ok := &goldverify.Envelope{Payload: goldverify.Payload{Type: goldverify.TypeApproval, TicketID: "GC-1", TicketSHA256: strings.Repeat("a", 64), Decision: "approve", TS: 1}}
	if err := s.Relay(ok); err != nil {
		t.Fatal(err)
	}
	tk, _ := s.Ticket("GC-1")
	if tk.Sent != "approve" {
		t.Fatal("sent state missing")
	}
	filepath.Walk(s.Dir, func(p string, _ os.FileInfo, _ error) error {
		if strings.HasSuffix(p, ".approval") {
			t.Fatal("console wrote an approval file")
		}
		return nil
	})
	if s.Relay(&goldverify.Envelope{Payload: goldverify.Payload{Type: goldverify.TypeRevoke, CID: "nope", Decision: "revoke", TS: 1}}) != ErrNotFound {
		t.Fatal("revoke of unknown cert relayed")
	}
}

func TestRegistration(t *testing.T) {
	s := newStore(t)
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	reg := func(ch, origin string) Registration {
		cd, _ := json.Marshal(map[string]string{"type": "webauthn.create", "challenge": ch, "origin": origin})
		return Registration{Label: "iPhone", CredentialID: "cred1", SPKI: goldverify.B64.EncodeToString(der), Alg: -7,
			ClientDataJSON: goldverify.B64.EncodeToString(cd)}
	}
	if _, err := s.FinishRegistration(reg("never-issued", s.Origins[0])); err == nil {
		t.Fatal("unknown challenge accepted")
	}
	if _, err := s.FinishRegistration(reg(s.BeginRegistration(), "https://evil.invalid")); err == nil {
		t.Fatal("wrong origin accepted")
	}
	ch := s.BeginRegistration()
	k, err := s.FinishRegistration(reg(ch, s.Origins[0]))
	if err != nil || k.Owner != "operator" || len(k.Fingerprint) != 64 {
		t.Fatal(err, k)
	}
	if _, err := s.FinishRegistration(reg(ch, s.Origins[0])); err == nil {
		t.Fatal("registration challenge reused")
	}
	// The stored file loads as a Gold pinned key (what the operator copies after comparing the fingerprint).
	b, _ := os.ReadFile(filepath.Join(s.Dir, "keys", k.KID+".json"))
	tmp := filepath.Join(t.TempDir(), "keys.json")
	os.WriteFile(tmp, []byte("["+string(b)+"]"), 0o644)
	if ks, err := goldverify.LoadKeys(tmp); err != nil || ks[0].KID != k.KID {
		t.Fatal(err)
	}
}

func TestBypassStatus(t *testing.T) {
	s := newStore(t)
	cert := func(cid string, nbf, exp int64) {
		writeJSON(filepath.Join(s.Dir, "certs", cid+".json"), Cert{CID: cid, HolderAgent: "node", Scope: []string{"gold:T1"}, NBF: nbf, Exp: exp})
	}
	if len(s.Bypass().Active) != 0 {
		t.Fatal("expected none")
	}
	n := now.Unix()
	cert("live", n-60, n+3600)
	cert("expired", n-7200, n-1)
	cert("future", n+60, n+3600)
	cert("revoked", n-60, n+3600)
	os.MkdirAll(filepath.Join(s.Dir, "revoked"), 0o755)
	os.WriteFile(filepath.Join(s.Dir, "revoked", "revoked"), nil, 0o644)
	st := s.Bypass()
	if len(st.Active) != 1 || st.Active[0].CID != "live" || st.Active[0].RevokeSent {
		t.Fatalf("%+v", st)
	}
	if err := s.Relay(&goldverify.Envelope{Payload: goldverify.Payload{Type: goldverify.TypeRevoke, CID: "live", Decision: "revoke", TS: 1}}); err != nil {
		t.Fatal(err)
	}
	if st := s.Bypass(); !st.Active[0].RevokeSent {
		t.Fatal("revoke pending must stay visible as active")
	}
}
