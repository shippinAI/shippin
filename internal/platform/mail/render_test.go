package mail

import (
	"net/mail"
	"strings"
	"testing"
	"time"
)

func mustRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestEveryKindRendersAndEscapes(t *testing.T) {
	r := mustRenderer(t)
	vars := map[string]string{"Brand": "Shippin", "Tenant": "Acme", "Workspace": "api", "Admin": "<script>x</script>", "Actor": "Ann\nB"}
	if len(r.Kinds()) != 7 {
		t.Fatalf("kinds: %v", r.Kinds())
	}
	for _, k := range r.Kinds() {
		out, err := r.Render(Notice{Kind: k, Vars: vars, Facts: [][2]string{{"Reason", "<b>hi</b>"}}, URL: "https://c.test", Footer: "f"})
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if strings.Contains(out.HTML, "<script>") || strings.Contains(out.HTML, "<b>hi") {
			t.Errorf("%s: HTML must escape user text", k)
		}
		if strings.ContainsAny(out.Subject, "\r\n") || out.Subject == "" {
			t.Errorf("%s: bad subject %q", k, out.Subject)
		}
		if !strings.Contains(out.Text, "https://c.test") {
			t.Errorf("%s: text part lacks the link", k)
		}
	}
	if _, err := r.Render(Notice{Kind: "access_request", Vars: map[string]string{"Brand": "x"}}); err == nil {
		t.Fatal("missing variables must be an error, not an empty string")
	}
}

func TestBuildRejectsHeaderInjection(t *testing.T) {
	from := mail.Address{Name: "Shippin", Address: "notices@shippin.test"}
	if _, err := Build(from, "a@example.com\r\nBcc: x@evil.test", Rendered{Subject: "s"}, time.Now()); err == nil {
		t.Fatal("recipient with CRLF must be refused")
	}
	raw, err := Build(from, "a@example.com", Rendered{Subject: "line1\nBcc: x@evil.test", Text: "t", HTML: "h"}, time.Now())
	if err != nil || strings.Contains(string(raw), "\nBcc:") {
		t.Fatal("subject newlines must not create headers")
	}
}
