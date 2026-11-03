package servers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// Prefixes make a leaked secret recognisable (and greppable by secret scanners).
const (
	TokenPrefix = "shpe_" // enrollment token (single use)
	CredPrefix  = "shph_" // host credential (bearer, renewed by heartbeats)
)

// newSecret returns prefix + 256 random bits (base64url) and its sha256 hex. Only the hash is stored.
func newSecret(prefix string) (plain, hash string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	plain = prefix + base64.RawURLEncoding.EncodeToString(b[:])
	return plain, Hash(plain), nil
}

// Hash is the stored form of a secret. High-entropy secrets need no slow KDF.
func Hash(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// wellFormed rejects obviously wrong input before any database lookup.
func wellFormed(plain, prefix string) bool {
	return strings.HasPrefix(plain, prefix) && len(plain) == len(prefix)+43
}
