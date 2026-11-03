package api

import (
	"os"
	"testing"
)

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWorkspaceHostFromPreview(t *testing.T) {
	cases := map[string]string{
		"5173--dev-03e90d.ws.example.test":  "dev-03e90d.ws.example.test",
		"65535--dev-03e90d.ws.example.test": "dev-03e90d.ws.example.test",
		"dev-03e90d.ws.example.test":        "dev-03e90d.ws.example.test",
		"80--dev.ws.example.test":           "80--dev.ws.example.test",     // ports < 1000 are never previews
		"123456--dev.ws.example.test":       "123456--dev.ws.example.test", // 6 digits: not a port
		"5173-dev.ws.example.test":          "5173-dev.ws.example.test",    // single dash: a workspace name
		"x5173--dev.ws.example.test":        "x5173--dev.ws.example.test",  // must be at the start
		"5173--5173--dev.ws.example.test":   "5173--dev.ws.example.test",   // only one prefix stripped
	}
	for in, want := range cases {
		if got := workspaceHost(in); got != want {
			t.Errorf("workspaceHost(%q) = %q, want %q", in, got, want)
		}
	}
}
