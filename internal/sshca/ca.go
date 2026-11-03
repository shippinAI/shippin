// Package sshca is the Shippin SSH user CA (N4 devspace access): it signs short-lived user
// certificates whose only principal is one workspace, so a cert opens exactly one devspace and
// expires on its own. The CA private key is read from a file and never logged or returned.
package sshca

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// CA holds the signer.
type CA struct{ signer ssh.Signer }

// Load reads an OpenSSH private key (e.g. from ssh-keygen -t ed25519) from path.
func Load(path string) (*CA, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ssh ca: %w", err)
	}
	s, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		return nil, errors.New("ssh ca: key file is not an unencrypted OpenSSH private key")
	}
	return &CA{signer: s}, nil
}

// New wraps an existing signer (tests).
func New(s ssh.Signer) *CA { return &CA{signer: s} }

// PublicKey is the line workspaces put in TrustedUserCAKeys.
func (c *CA) PublicKey() string { return string(ssh.MarshalAuthorizedKey(c.signer.PublicKey())) }

// Request describes one certificate.
type Request struct {
	PublicKey string        // the client's authorized_keys line (ephemeral key)
	Principal string        // workspace principal, e.g. w-<uuid>
	KeyID     string        // audit identity: account uuid + email + session
	TTL       time.Duration // short; the cert is useless after it
}

// Sign returns the certificate line (ssh-…-cert-v01@openssh.com …) and its expiry.
func (c *CA) Sign(r Request) (string, time.Time, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(r.PublicKey))
	if err != nil {
		return "", time.Time{}, errors.New("public_key is not an OpenSSH public key")
	}
	if r.Principal == "" || r.TTL <= 0 {
		return "", time.Time{}, errors.New("principal and ttl are required")
	}
	var serial [8]byte
	if _, err := rand.Read(serial[:]); err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	until := now.Add(r.TTL)
	cert := &ssh.Certificate{
		Key: pub, Serial: binary.BigEndian.Uint64(serial[:]), CertType: ssh.UserCert, KeyId: r.KeyID,
		ValidPrincipals: []string{r.Principal},
		ValidAfter:      uint64(now.Add(-time.Minute).Unix()), ValidBefore: uint64(until.Unix()),
		Permissions: ssh.Permissions{Extensions: map[string]string{"permit-pty": "", "permit-port-forwarding": ""}},
	}
	if err := cert.SignCert(rand.Reader, c.signer); err != nil {
		return "", time.Time{}, err
	}
	return string(ssh.MarshalAuthorizedKey(cert)), until, nil
}
