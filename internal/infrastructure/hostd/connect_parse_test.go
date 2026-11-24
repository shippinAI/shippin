package hostd

import "testing"

func TestLoginPrompt(t *testing.T) {
	gh := ConnectProvider{URLPrefixes: []string{"https://github.com/login/device"}, CodePattern: `one-time code: ([A-Z0-9]{4}-[A-Z0-9]{4})`}
	out := "\x1b[1m! First copy your one-time code: AB12-CD34\x1b[0m\nOpen https://github.com/login/device in your browser.\nhttps://evil.example/x"
	u, c := loginPrompt(gh, out)
	if u != "https://github.com/login/device" || c != "AB12-CD34" {
		t.Fatalf("got %q %q", u, c)
	}
	if u, c := loginPrompt(gh, "token gho_secret https://evil.example"); u != "" || c != "" {
		t.Fatalf("leaked %q %q", u, c)
	}
	if !pasteCodeRe.MatchString("abc#DEF_12") || pasteCodeRe.MatchString("a b") || pasteCodeRe.MatchString("$(rm)") {
		t.Fatal("paste code filter")
	}
}
