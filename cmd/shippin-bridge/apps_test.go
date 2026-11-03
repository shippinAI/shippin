package main

import (
	"bytes"
	"strings"
	"testing"

	"shippin.cloud/console/internal/apps"
)

func TestAppsPrintIsOfficial(t *testing.T) {
	var b bytes.Buffer
	if err := appsPrint(&b, "claude-code"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "https://claude.ai/install.sh") || !strings.Contains(b.String(), "exec claude") || strings.Contains(b.String(), "sudo") {
		t.Fatal(b.String())
	}
	b.Reset()
	if err := appsPrint(&b, "claude-code-apt"); err != nil || !strings.Contains(b.String(), apps.ClaudeKeyFingerprint) || !strings.Contains(b.String(), "sudo ") {
		t.Fatal(b.String(), err)
	}
	b.Reset()
	if err := appsPrint(&b, "claude-code-update"); err != nil || !strings.Contains(b.String(), "claude update") || strings.Contains(b.String(), "sudo") || !strings.Contains(b.String(), "automatically update in the background") {
		t.Fatal(b.String(), err)
	}
	b.Reset()
	if err := appsPrint(&b, "claude-code-apt-update"); err != nil || !strings.Contains(b.String(), "--only-upgrade claude-code") {
		t.Fatal(b.String(), err)
	}
	if strings.Contains(b.String(), "ANTHROPIC_API_KEY=") || strings.Contains(b.String(), "ANTHROPIC_BASE_URL=") {
		t.Fatal(b.String())
	}
	b.Reset()
	if err := appsPrint(&b, "claude-desktop"); err != nil || !strings.Contains(b.String(), "claude-desktop") {
		t.Fatal(b.String(), err)
	}
	b.Reset()
	if err := appsPrint(&b, "codex"); err != nil || !strings.Contains(b.String(), "https://chatgpt.com/codex/install.sh") || !strings.Contains(b.String(), "exec codex") {
		t.Fatal(b.String(), err)
	}
	if err := appsPrint(&b, "grok"); err == nil || strings.Contains(err.Error(), "curl") {
		t.Fatalf("grok: %v", err)
	}
	b.Reset()
	if err := appsPrint(&b, "chatgpt"); err != nil || !strings.Contains(b.String(), "chatgpt_amd64.deb") || strings.Contains(b.String(), "sha256sum") || !strings.Contains(b.String(), "sudo apt-get install") {
		t.Fatal(b.String(), err)
	}
	b.Reset()
	if err := appsPrint(&b, "chatgpt-update"); err != nil || !strings.Contains(b.String(), "--only-upgrade chatgpt") || strings.Contains(b.String(), "sha256sum") {
		t.Fatal(b.String(), err)
	}
	b.Reset()
	if err := appsPrint(&b, "codex-update"); err != nil || strings.Contains(b.String(), "exec codex") || strings.Contains(b.String(), "sudo") {
		t.Fatal(b.String(), err)
	}
}

func TestDesktopURL(t *testing.T) {
	got := desktopURL("https://console.example/", "00000000-0000-4000-8000-000000000001")
	want := "https://console.example/t/00000000-0000-4000-8000-000000000001/desktop"
	if got != want {
		t.Fatal(got)
	}
}
