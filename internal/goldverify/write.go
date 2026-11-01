package goldverify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// writeApproval writes <id>.approval in exactly gold-approve's line format and field order, owned
// by the approver, mode 0644, via tmp + rename (gc_check_approval accepts it unchanged).
func (c *Config) writeApproval(p Payload, kid string, now time.Time) (string, error) {
	dec, reason := "approved", "passkey approval ("+kid+")"
	if p.Decision == "deny" {
		dec, reason = "rejected", "passkey denial ("+kid+")"
	}
	body := "decision: " + dec + "\n" +
		"approved_by: " + c.Approver + "\n" +
		"identity: " + c.Approver + "@passkey:" + kid + "\n" +
		"ticket_id: " + p.TicketID + "\n" +
		"ticket_sha256: " + p.TicketSHA256 + "\n" +
		"approved_at: " + strconv.FormatInt(now.Unix(), 10) + "\n" +
		"reason: " + reason + "\n"
	f := filepath.Join(c.ApprovalDir, p.TicketID+".approval")
	return f, c.atomicWrite(f, body)
}

func (c *Config) writeRevocation(p Payload, kid string, now time.Time) (string, error) {
	dir := filepath.Join(c.StateDir, "revoked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f := filepath.Join(dir, p.CID)
	body := fmt.Sprintf("cid: %s\nrevoked_by: %s\nidentity: %s@passkey:%s\nrevoked_at: %d\n", p.CID, c.Approver, c.Approver, kid, now.Unix())
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return "", err
	}
	return f, os.Rename(tmp, f)
}

func (c *Config) atomicWrite(f, body string) error {
	tmp := f + ".tmp"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	if c.ApproverUID >= 0 {
		if err := os.Lchown(tmp, c.ApproverUID, c.ApproverGID); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f)
}

// logDecision appends to decisions.jsonl with gc_audit's key order; failures are logged too.
func (c *Config) logDecision(e *Envelope, r Result) {
	p := e.Payload
	ev := map[string]string{"approve": "approve", "deny": "reject", "revoke": "revoke"}[p.Decision]
	res := map[string]string{"approve": "approved", "deny": "rejected", "revoke": "revoked"}[p.Decision]
	detail := "passkey " + e.KID
	if !r.OK {
		ev, res, detail = "passkey-refused", "unverified-passkey", r.Reason
	}
	id := p.TicketID
	if id == "" {
		id = p.CID
	}
	entry := struct {
		TS     string `json:"ts"`
		Source string `json:"source"`
		Event  string `json:"event"`
		ID     string `json:"id"`
		Result string `json:"result"`
		Detail string `json:"detail"`
		User   string `json:"user"`
	}{c.now().Format(time.RFC3339), "gold-passkey", ev, id, res, detail, c.Approver}
	b, _ := json.Marshal(entry)
	log := filepath.Join(c.ApprovalDir, "decisions.jsonl")
	fh, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	fh.Write(append(b, '\n'))
	if c.ApproverUID >= 0 {
		_ = os.Lchown(log, c.ApproverUID, c.ApproverGID)
	}
}
