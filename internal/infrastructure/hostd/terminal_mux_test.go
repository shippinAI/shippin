package hostd

import (
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestTermMuxIsAFixedToken(t *testing.T) {
	for q, want := range map[string]string{"": "zellij", "mux=plain": "plain", "mux=zellij": "zellij", "mux=;rm%20-rf": "zellij"} {
		if got := termMux(httptest.NewRequest("GET", "/x?"+q, nil)); got != want {
			t.Errorf("%q: got %q want %q", q, got, want)
		}
	}
}

// The script prefers shippin-term (Zellij) and falls back to bash -l on older images.
func TestTermScriptFallsBackWithoutShippinTerm(t *testing.T) {
	out, err := exec.Command("sh", "-c", strings.Replace(termScript, "exec bash -l", "echo fallback:$3", 1), "term", "24", "80", "zellij").Output()
	if err != nil || strings.TrimSpace(string(out)) != "fallback:zellij" {
		t.Fatalf("got %q %v", out, err)
	}
}
