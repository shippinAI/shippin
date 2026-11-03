package chat

import (
	"context"
	"io"
	"strings"
	"testing"

	"shippin.cloud/console/internal/config"
)

type fakeRun struct{ out string }

func (f fakeRun) ChatRun(context.Context, string, string, string, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.out)), nil
}

func TestTurnParsesClaudeStreamAndA2UI(t *testing.T) {
	out := `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Hello "}]}}
{"type":"result","result":"Hello there\n` + "```a2ui\\n{\\\"type\\\":\\\"modal\\\",\\\"title\\\":\\\"Deploy?\\\",\\\"body\\\":\\\"Ship it\\\",\\\"items\\\":[{\\\"type\\\":\\\"button\\\",\\\"label\\\":\\\"Yes\\\",\\\"prompt\\\":\\\"deploy\\\"}]}\\n```" + `"}
{"shippin_exit":0,"stderr":""}
`
	p := config.ChatProvider{Name: "Claude", Command: "c", Format: "claude-stream-json"}
	var streamed string
	r := Turn(context.Background(), fakeRun{out}, config.Chat{MaxReplyBytes: 4096}, p, "t", "w", "s", "hi", func(s string) { streamed += s })
	if r.Err != "" || r.Text != "Hello there" || streamed != "Hello " {
		t.Fatalf("%+v streamed=%q", r, streamed)
	}
	if len(r.Blocks) != 1 || r.Blocks[0].Type != "modal" || len(r.Blocks[0].Items) != 1 {
		t.Fatalf("blocks %+v", r.Blocks)
	}
}

func TestTurnReportsCLIFailure(t *testing.T) {
	p := config.ChatProvider{Name: "Claude", Format: "claude-stream-json"}
	r := Turn(context.Background(), fakeRun{`{"shippin_exit":127,"stderr":"claude: command not found"}` + "\n"}, config.Chat{MaxReplyBytes: 4096}, p, "t", "w", "s", "hi", func(string) {})
	if !strings.Contains(r.Err, "command not found") {
		t.Fatal(r)
	}
}

func TestValidateDropsUnsafe(t *testing.T) {
	if _, ok := Validate(Block{Type: "script", Body: "x"}, 0); ok {
		t.Fatal("unknown type accepted")
	}
	b, ok := Validate(Block{Type: "button", Label: "Go", Href: "javascript:alert(1)"}, 0)
	if ok || b.Href != "" {
		t.Fatal("unsafe href kept")
	}
	f, ok := Validate(Block{Type: "form", Prompt: "p", Fields: []Field{{Name: "Bad Name", Kind: "text"}, {Name: "repo", Kind: "text"}}}, 0)
	if !ok || len(f.Fields) != 1 {
		t.Fatal(f)
	}
}
