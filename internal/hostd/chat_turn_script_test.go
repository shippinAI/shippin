package hostd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The in-workspace wrapper keeps the CLI session id in rrflow memory and resumes it next turn.
func TestChatTurnScriptRRFlow(t *testing.T) {
	script, _ := filepath.Abs("../../deploy/image/ai/shippin-chat-turn")
	bin, mem := t.TempDir(), t.TempDir()
	write := func(n, body string) {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/bash\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// fake claude: prints its argv and a session id; fake rrflow: memory in files, logs decisions.
	write("claude", `cat >/dev/null; echo "{\"type\":\"system\",\"session_id\":\"0a0b0c0d-0000-4000-8000-000000000001\",\"argv\":\"$*\"}"; echo '{"type":"result","result":"pong"}'`)
	write("rrflow", `M=`+mem+`; k=$(echo "$3" | tr / _)
case "$1 $2" in
"memory get") [ -f "$M/$k" ] && echo "{\"key\":\"$3\",\"value\":\"$(cat "$M/$k")\"}" || exit 1;;
"memory put") echo -n "$4" > "$M/$k";;
"memory rm") rm -f "$M/$k";;
"session start") echo 11111111-2222-4333-8444-555555555555;;
"decision add") echo "$*" >> "$M/decisions";;
esac`)
	ses := "019a0000-0000-7000-8000-0000000000aa"
	run := func() string {
		c := exec.Command("bash", script, "claude", ses)
		c.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "RRFLOW_URL=http://rr.test", "HOME="+t.TempDir())
		c.Stdin = strings.NewReader("hi")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}
	if out := run(); strings.Contains(out, "--resume") {
		t.Fatalf("first turn must not resume: %s", out)
	}
	if b, _ := os.ReadFile(filepath.Join(mem, "chat_"+ses+"_claude")); string(b) != "0a0b0c0d-0000-4000-8000-000000000001" {
		t.Fatalf("CLI session not stored in rrflow: %q", b)
	}
	if out := run(); !strings.Contains(out, "--resume 0a0b0c0d-0000-4000-8000-000000000001") {
		t.Fatalf("second turn must resume via rrflow: %s", out)
	}
	d, _ := os.ReadFile(filepath.Join(mem, "decisions"))
	if strings.Count(string(d), "--action chat.turn --effect allow") != 2 || !strings.Contains(string(d), "--session 11111111-2222-4333-8444-555555555555") {
		t.Fatalf("decisions: %s", d)
	}
}
