package state

import (
	"os"
	"path/filepath"
	"testing"
)

const org, ws, other = "019a0000-0000-7000-8000-00000000f001", "ws-a", "ws-b"

func TestStatePersistsAndChains(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, org)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.StartSession(ws, "claude", "run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutMemory("workspace:"+ws, "stack", "vite"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutMemory("workspace:"+ws, "stack", "vite+react"); err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"allow", "deny"} {
		if _, err := s.AppendDecision(Decision{Workspace: ws, Session: sess.ID, Actor: "agent:x", Action: "tool.call", Effect: e}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AppendDecision(Decision{Workspace: other, Session: sess.ID, Actor: "a", Action: "tool.call", Effect: "allow"}); err != ErrNotFound {
		t.Fatal("another workspace logged against this session")
	}
	if _, err := s.AppendDecision(Decision{Workspace: ws, Actor: "a", Action: "tool.call", Effect: "maybe"}); err != ErrInvalid {
		t.Fatal("unknown effect accepted")
	}
	if _, err := s.EndSession(other, sess.ID, "done"); err != ErrNotFound {
		t.Fatal("another workspace ended this session")
	}
	_, _ = s.EndSession(ws, sess.ID, "exit-0")
	r, err := Open(dir, org)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := r.GetMemory("workspace:"+ws, "stack"); m == nil || m.Value != "vite+react" || m.Version != 2 {
		t.Fatalf("memory after reload: %+v", m)
	}
	if ss := r.Sessions(ws, 10); len(ss) != 1 || ss[0].Status != "exit-0" || ss[0].EndedAt == nil {
		t.Fatalf("sessions after reload: %+v", ss)
	}
	if n, err := r.Verify(); err != nil || n != 2 {
		t.Fatalf("chain %d %v", n, err)
	}
	if len(r.Memory("workspace:"+other, 10)) != 0 || len(r.Decisions(other, 10)) != 0 {
		t.Fatal("workspace scoping leaked")
	}
}

func TestTamperAndTornLine(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, org)
	for i := 0; i < 3; i++ {
		_, _ = s.AppendDecision(Decision{Workspace: ws, Actor: "a", Action: "model.call", Effect: "allow"})
	}
	p := filepath.Join(dir, decisionsFile)
	raw, _ := os.ReadFile(p)
	_ = os.WriteFile(p, append(raw, []byte(`{"seq":4,"hash":"tor`)...), 0o600) // crash mid-append
	r, err := Open(dir, org)
	if err != nil || len(r.Decisions("", 10)) != 3 {
		t.Fatalf("torn last line must be cut: %v", err)
	}
	if _, err := r.AppendDecision(Decision{Workspace: ws, Actor: "a", Action: "model.call", Effect: "deny"}); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Verify(); err != nil || n != 4 {
		t.Fatalf("chain after torn line: %d %v", n, err)
	}
	r.decisions[1].Effect = "deny" // tamper
	if _, err := r.Verify(); err == nil {
		t.Fatal("tampered decision not detected")
	}
}
