package hostd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func chatServer() *Server {
	s := testServer()
	s.cfg.Chat = ChatConfig{User: "dev", MaxPromptBytes: 64, TimeoutSeconds: 5, SessionCommands: []string{"claude-turn"},
		Commands: map[string][]string{"claude-turn": {"shippin-chat-turn", "claude"}, "claude-check": {"sh", "-c", "exit 3"}}}
	return s
}

func TestChatArgv(t *testing.T) {
	c := chatServer().cfg.Chat
	sid := "019a0000-0000-7000-8000-0000000000aa"
	if got, ok := c.chatArgv("claude-turn", sid); !ok || !slices.Equal(got, []string{"shippin-chat-turn", "claude", sid}) {
		t.Fatalf("session command: %v %v", got, ok)
	}
	for _, bad := range []string{"", "x; rm -rf ~", "../etc", sid + " --resume"} {
		if got, _ := c.chatArgv("claude-turn", bad); got[len(got)-1] != "-" {
			t.Fatalf("unvalidated session %q passed: %v", bad, got)
		}
	}
	if got, _ := c.chatArgv("claude-check", sid); len(got) != 3 {
		t.Fatalf("non-session command must not get the session: %v", got)
	}
	if _, ok := c.chatArgv("bash", sid); ok {
		t.Fatal("unknown command accepted")
	}
	got, _ := c.chatArgv("claude-turn", sid)
	got[0] = "mutated"
	if c.Commands["claude-turn"][0] != "shippin-chat-turn" {
		t.Fatal("chatArgv aliases the config slice")
	}
}

func TestChatIncusArgs(t *testing.T) {
	got := chatIncusArgs(wsid, "dev", []string{"claude", "-p", "--output-format", "stream-json"})
	want := []string{"exec", "--force-noninteractive", "w-" + wsid, "--", "su", "-", "dev", "-s", "/bin/bash",
		"-c", `exec "$0" "$@"`, "--", "claude", "-p", "--output-format", "stream-json"}
	if !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
}

func TestStreamChatTrailer(t *testing.T) {
	var b strings.Builder
	streamChat(context.Background(), &b, "sh", []string{"-c", `cat; echo '{"type":"result"}'; echo oops >&2; exit 3`}, []byte("hi\n"))
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 3 || lines[0] != "hi" || lines[1] != `{"type":"result"}` {
		t.Fatalf("stdout not streamed through: %q", lines)
	}
	var tr struct {
		Exit   int    `json:"shippin_exit"`
		Stderr string `json:"stderr"`
	}
	if json.Unmarshal([]byte(lines[2]), &tr) != nil || tr.Exit != 3 || tr.Stderr != "oops\n" {
		t.Fatalf("trailer %q", lines[2])
	}
	b.Reset()
	streamChat(context.Background(), &b, "sh", []string{"-c", `head -c 10000 /dev/zero | tr '\0' x >&2`}, nil)
	if json.Unmarshal([]byte(strings.TrimSpace(b.String())), &tr) != nil || len(tr.Stderr) > 4096 {
		t.Fatalf("stderr not bounded: %d", len(tr.Stderr))
	}
}

func TestChatExecRejects(t *testing.T) {
	s := chatServer()
	for _, tc := range []struct {
		cmd, body string
		code      int
	}{{"rm", "x", http.StatusNotFound}, {"claude-turn", strings.Repeat("a", 65), http.StatusRequestEntityTooLarge}} {
		r := httptest.NewRequest("POST", "/tenants/"+tid+"/workspaces/"+wsid+"/chat/"+tc.cmd, strings.NewReader(tc.body))
		r.SetPathValue("t", tid)
		r.SetPathValue("w", wsid)
		r.SetPathValue("cmd", tc.cmd)
		w := httptest.NewRecorder()
		s.chatExec(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s: got %d want %d", tc.cmd, w.Code, tc.code)
		}
	}
}
