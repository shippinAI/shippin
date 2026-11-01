package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func tokenEqual(raw string, sum []byte) bool {
	got := hashToken(raw)
	return len(sum) == len(got) && subtle.ConstantTimeCompare(got, sum) == 1
}

// NewToken is a URL-safe random string for login state.
func NewToken() (string, error) { return randomToken(18) }

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func emailNorm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// PKCEChallenge is the S256 challenge for a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// PKCEMatch reports whether verifier satisfies an S256 challenge.
func PKCEMatch(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(PKCEChallenge(verifier)), []byte(challenge)) == 1
}

func hexHash(raw string) string { return hex.EncodeToString(hashToken(raw)) }
