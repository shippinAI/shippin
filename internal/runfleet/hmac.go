package runfleet

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func hmacSHA256(key, body []byte) string {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func hmacEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
