package main

import (
	"crypto/hmac"
	"crypto/sha256"
)

func runfleetEqual(a, b string) bool {
	sumA := sha256.Sum256([]byte(a))
	sumB := sha256.Sum256([]byte(b))
	return hmac.Equal(sumA[:], sumB[:])
}
