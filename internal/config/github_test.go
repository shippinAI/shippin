package config

import (
	"encoding/base64"
	"testing"
)

func TestPemOrBase64(t *testing.T) {
	pem := "-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----\n"
	for in, want := range map[string]string{
		"":  "",
		pem: pem,
		base64.StdEncoding.EncodeToString([]byte(pem)): pem,
		"not-a-key": "not-a-key",
		base64.StdEncoding.EncodeToString([]byte("x")): base64.StdEncoding.EncodeToString([]byte("x")),
	} {
		if got := pemOrBase64(in); got != want {
			t.Fatalf("pemOrBase64(%q) = %q, want %q", in, got, want)
		}
	}
}
