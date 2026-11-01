// Package bridge stores device sign-ins and bridge sessions (022_bridge.sql). Device codes and
// bearer tokens exist in clear only in the response that hands them out; the table keeps hashes.
package bridge

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// userAlphabet avoids look-alike characters (no 0/O, 1/I/L) for codes people type.
const userAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return b
}

// secret is a 256-bit URL-safe random string with prefix.
func secret(prefix string) string { return prefix + base64.RawURLEncoding.EncodeToString(random(32)) }

// UserCode is XXXX-XXXX from userAlphabet.
func userCode() string {
	b := random(8)
	var s strings.Builder
	for i, c := range b {
		if i == 4 {
			s.WriteByte('-')
		}
		s.WriteByte(userAlphabet[int(c)%len(userAlphabet)])
	}
	return s.String()
}

// NormalizeUserCode upper-cases and re-inserts the dash so "abcd efgh" matches.
func NormalizeUserCode(in string) string {
	var s strings.Builder
	for _, r := range strings.ToUpper(in) {
		if strings.ContainsRune(userAlphabet, r) {
			s.WriteRune(r)
		}
	}
	c := s.String()
	if len(c) != 8 {
		return ""
	}
	return c[:4] + "-" + c[4:]
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
