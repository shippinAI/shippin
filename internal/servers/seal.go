package servers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Sealer encrypts hypervisor secrets at rest. The key is derived from the panel's session key with a
// purpose of its own, and each secret is bound to its server, so one row's ciphertext cannot be moved
// onto another. Replacing the session key makes sealed secrets unreadable: re-enter them.
type Sealer struct{ aead cipher.AEAD }

// NewSealer derives the key from SHIPPIN_SESSION_KEY.
func NewSealer(hexKey string) (*Sealer, error) {
	root, err := hex.DecodeString(hexKey)
	if err != nil || len(root) < 16 {
		return nil, errors.New("servers: the panel has no usable session key, so it cannot store secrets")
	}
	key, err := hkdf.Key(sha256.New, root, nil, "shippin hypervisor secrets v1", 32)
	if err != nil {
		return nil, err
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plain for the server with this id.
func (s *Sealer) Seal(plain []byte, serverID string) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plain, []byte(serverID)), nil
}

// Open decrypts what Seal produced for the same server.
func (s *Sealer) Open(box []byte, serverID string) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(box) < n {
		return nil, errors.New("servers: stored secret is damaged")
	}
	plain, err := s.aead.Open(nil, box[:n], box[n:], []byte(serverID))
	if err != nil {
		return nil, errors.New("servers: stored secret cannot be read; the session key may have changed, so enter it again")
	}
	return plain, nil
}
