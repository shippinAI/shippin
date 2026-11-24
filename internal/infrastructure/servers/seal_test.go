package servers

import (
	"bytes"
	"strings"
	"testing"
)

var testKey = strings.Repeat("cd", 32)

func TestSealRoundTripAndBindsToTheServer(t *testing.T) {
	s, err := NewSealer(testKey)
	if err != nil {
		t.Fatal(err)
	}
	box, err := s.Seal([]byte("token-secret"), "server-a")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(box, []byte("token-secret")) {
		t.Fatal("stored in the clear")
	}
	got, err := s.Open(box, "server-a")
	if err != nil || string(got) != "token-secret" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := s.Open(box, "server-b"); err == nil {
		t.Fatal("a secret opened for a different server")
	}
	again, _ := s.Seal([]byte("token-secret"), "server-a")
	if bytes.Equal(box, again) {
		t.Fatal("same ciphertext twice")
	}
}

func TestSealFailsClearlyWithAnotherKeyOrNoKey(t *testing.T) {
	s, _ := NewSealer(testKey)
	box, _ := s.Seal([]byte("x"), "a")
	other, _ := NewSealer(strings.Repeat("ab", 32))
	if _, err := other.Open(box, "a"); err == nil || !strings.Contains(err.Error(), "enter it again") {
		t.Fatalf("%v", err)
	}
	for _, bad := range []string{"", "nothex", "abcd"} {
		if _, err := NewSealer(bad); err == nil {
			t.Errorf("accepted key %q", bad)
		}
	}
	if _, err := s.Open([]byte{1, 2}, "a"); err == nil {
		t.Fatal("short input accepted")
	}
}
