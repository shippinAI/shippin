package vaultixhost

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// Sealer encrypts the stored console client secrets (AES-256-GCM, nonce prepended).
type Sealer struct{ g cipher.AEAD }

// NewSealer takes a 64-hex-char key from the secrets store.
func NewSealer(hexKey string) (*Sealer, error) {
	k, err := hex.DecodeString(hexKey)
	if err != nil || len(k) != 32 {
		return nil, errors.New("vaultixhost: seal key must be 32 bytes hex")
	}
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	return &Sealer{g: g}, err
}

func (s *Sealer) Seal(plain, sub string) ([]byte, error) {
	n := make([]byte, s.g.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return nil, err
	}
	return s.g.Seal(n, n, []byte(plain), []byte(sub)), nil
}

func (s *Sealer) Open(box []byte, sub string) (string, error) {
	ns := s.g.NonceSize()
	if len(box) < ns {
		return "", errors.New("vaultixhost: bad seal")
	}
	p, err := s.g.Open(nil, box[:ns], box[ns:], []byte(sub))
	return string(p), err
}
