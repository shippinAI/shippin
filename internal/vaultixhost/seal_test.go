package vaultixhost

import (
	"strings"
	"testing"
)

func TestSealBindsToSubject(t *testing.T) {
	s, err := NewSealer(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	box, err := s.Seal("client-secret", "sub-a")
	if err != nil {
		t.Fatal(err)
	}
	if v, err := s.Open(box, "sub-a"); err != nil || v != "client-secret" {
		t.Fatalf("round trip: %q %v", v, err)
	}
	if _, err := s.Open(box, "sub-b"); err == nil {
		t.Fatal("a sealed secret must not open for another person")
	}
	if _, err := NewSealer("short"); err == nil {
		t.Fatal("short key accepted")
	}
}
