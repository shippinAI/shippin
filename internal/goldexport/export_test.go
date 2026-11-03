package goldexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/goldsign"
)

const ticket = "id: GC-X1\ntitle: open 8099 to the tailnet\ncommands:\n  - nft add rule x\n"

// review prints exactly what cmd/gold-policy/review.go prints.
func review(id, sha string) string {
	short := sha
	if len(short) > 16 {
		short = sha[:16]
	}
	return fmt.Sprintf("REVIEW %s\nsha256: %s\ntier: T2 (ruleset v2)\n  tier because: firewall change\nclasses: firewall, net\n"+
		"checkset: base\nrevert_after_s: 300\ntargets: gold\noob_check (1):\n   1  NONE - reason: shared LOM\ncommands (2):\n"+
		"   1  nft add rule inet filter input tcp dport 8099 accept\n   2  nft list ruleset\nrollback (1):\n   1  nft delete rule x\n"+
		"verification (1):\n   1  gateway\ngate: approval required\napprove by chat (T2 only): \"approve %s %s\"\n", id, sha, id, short)
}

func TestParseReview(t *testing.T) {
	sha := strings.Repeat("c", 64)
	f, err := ParseReview(review("GC-X1", sha))
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != "GC-X1" || f.SHA256 != sha || f.Tier != "T2" || f.Host != "gold" || len(f.Commands) != 2 ||
		f.Commands[0] != "nft add rule inet filter input tcp dport 8099 accept" || f.OOBPath != "NONE - reason: shared LOM" ||
		len(f.Classes) != 2 || len(f.TierReasons) != 1 || len(f.Verification) != 1 {
		t.Fatalf("%+v", f)
	}
	if _, err := ParseReview(strings.Replace(review("GC-X1", sha), "commands (2)", "commands (3)", 1)); err == nil {
		t.Fatal("count mismatch accepted")
	}
	if _, err := ParseReview("REVIEW x\n"); err == nil {
		t.Fatal("incomplete accepted")
	}
}

func gold(t *testing.T) *Env {
	dir := t.TempDir()
	for _, d := range []string{"tickets", "approvals", "state/revoked", "state/used", "certs"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "tickets", "GC-X1.md"), []byte(ticket), 0o644)
	os.WriteFile(filepath.Join(dir, "tickets", "GC-X2.md"), []byte(ticket+"# v2\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "approvals", "GC-X2.approval"), []byte("decision: approved\nticket_sha256: deadbeef\n"), 0o644)
	bin := filepath.Join(dir, "gold-policy")
	script := "#!/bin/sh\nf=" + filepath.Join(dir, "tickets") + "/$2.md\ns=$(sha256sum $f | cut -d' ' -f1)\ncat <<EOF\n" + review("$2", "$s") + "EOF\n"
	os.WriteFile(bin, []byte(script), 0o755)
	os.WriteFile(filepath.Join(dir, "certs", "c1.json"), []byte(`{"cid":"c1","holder_agent":"node","scope":["gold:T1"],"nbf":1,"exp":9999999999,"max_uses":3,"uses":1,"envelope":"SECRET-ISH"}`), 0o600)
	os.WriteFile(filepath.Join(dir, "state/revoked", "c0"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "state/used", "ab"), nil, 0o600)
	return &Env{PolicyBin: bin, TicketDir: filepath.Join(dir, "tickets"), ApprovalDir: filepath.Join(dir, "approvals"),
		StateDir: filepath.Join(dir, "state"), CertDir: filepath.Join(dir, "certs"), Now: time.Now}
}

func TestCollectAndSyncIntoConsole(t *testing.T) {
	e := gold(t)
	// GC-X1 approved for an OLD hash stays pending; an approval for the current hash would hide it.
	os.WriteFile(filepath.Join(e.ApprovalDir, "GC-X1.approval"), []byte("decision: approved\nticket_sha256: "+strings.Repeat("0", 64)+"\n"), 0o644)
	s, errs := e.Collect(context.Background())
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(s.Facts) != 2 || s.Facts[0].AgentSummary != "open 8099 to the tailnet" || !s.CertsOK || len(s.Certs) != 1 || s.Revoked[0] != "c0" || s.Used[0] != "ab" {
		t.Fatalf("%+v", s)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "SECRET-ISH") {
		t.Fatal("cert store extras leaked")
	}
	// End to end into the console store through the real HTTP shape.
	cdir := t.TempDir()
	store := &goldsign.Store{Dir: cdir, FactsDir: filepath.Join(cdir, "facts")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		var in goldsign.Sync
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil || store.ApplySync(&in) != nil {
			w.WriteHeader(400)
			return
		}
	}))
	defer srv.Close()
	if err := (&Client{Base: srv.URL, Token: "tok", HTTP: srv.Client()}).Push(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if p, _ := store.Pending(); len(p) != 2 {
		t.Fatalf("console pending %d", len(p))
	}
	if st := store.Bypass(); !st.Known || len(st.Active) != 1 || st.Active[0].HolderAgent != "node" {
		t.Fatalf("%+v", st)
	}
	if (&Client{Base: srv.URL, Token: "wrong", HTTP: srv.Client()}).Push(context.Background(), s) == nil {
		t.Fatal("wrong token accepted")
	}
}

func TestDecidedTicketLeftOutAndUnreadableCertStore(t *testing.T) {
	e := gold(t)
	b, _ := os.ReadFile(filepath.Join(e.TicketDir, "GC-X1.md"))
	sum := sha256.Sum256(b)
	os.WriteFile(filepath.Join(e.ApprovalDir, "GC-X1.approval"), []byte("decision: approved\nticket_sha256: "+hex.EncodeToString(sum[:])+"\napproved_at: 1\n"), 0o644)
	os.WriteFile(filepath.Join(e.CertDir, "bad.json"), []byte("{"), 0o644)
	s, _ := e.Collect(context.Background())
	for _, f := range s.Facts {
		if f.ID == "GC-X1" {
			t.Fatal("decided ticket exported as pending")
		}
	}
	if s.CertsOK {
		t.Fatal("corrupt cert store must report certs_ok=false")
	}
}
