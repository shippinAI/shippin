package runfleet

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	ghp := "ghp_" + strings.Repeat("a", 36)
	aws := "AKIA" + strings.Repeat("A", 16)
	pem := "-----BEGIN OPENSSH PRIVATE KEY-----\nsecret-line\n-----END OPENSSH PRIVATE KEY-----"
	in := strings.Join([]string{
		"build started",
		"using " + ghp,
		"password=hunter2",
		"token count is fine",
		"api_key: " + aws,
		"Authorization: Bearer supersecret",
		pem,
	}, "\n")
	out, n := Redact(in)
	if n < 5 {
		t.Fatalf("replacements = %d, want at least 5\n%s", n, out)
	}
	for _, leak := range []string{ghp, "hunter2", aws, "supersecret", "secret-line"} {
		if strings.Contains(out, leak) {
			t.Fatalf("redacted log still contains %q\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "token count is fine") {
		t.Fatalf("ordinary token wording was removed:\n%s", out)
	}
	if !strings.Contains(out, "password="+redacted) {
		t.Fatalf("assignment key was not kept:\n%s", out)
	}
}
