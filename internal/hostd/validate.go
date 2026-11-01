// Package hostd is the narrowly scoped privileged helper on the Incus host.
// It only performs tenant-scoped operations on validated UUIDs; no free-form commands.
package hostd

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	uuidRe  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	labelRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	keyRe   = regexp.MustCompile(`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(256|384|521)|sk-ssh-ed25519@openssh\.com) [A-Za-z0-9+/=]{40,800}( [\w.@+-]{0,100})?$`)
)

func ValidUUID(s string) error {
	if !uuidRe.MatchString(s) {
		return fmt.Errorf("invalid uuid")
	}
	return nil
}

func ValidLabel(s string) error {
	if !labelRe.MatchString(s) {
		return fmt.Errorf("name must be 1-31 chars: lowercase letters, digits, dashes; start with a letter")
	}
	return nil
}

// ValidKeys accepts 0-20 OpenSSH public keys and returns them newline-joined.
func ValidKeys(keys []string) (string, error) {
	if len(keys) > 20 {
		return "", fmt.Errorf("at most 20 SSH public keys")
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if !keyRe.MatchString(k) {
			return "", fmt.Errorf("not an OpenSSH public key")
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		return "", nil
	}
	return strings.Join(out, "\n") + "\n", nil
}

func ValidPort(p, lo, hi int) error {
	if p < lo || p > hi {
		return fmt.Errorf("port out of range")
	}
	return nil
}

// Names derived from UUIDs (07: t-, p-, n-, w-, h- prefixes).
func Project(t string) string       { return "t-" + t }
func Pool(t string) string          { return "p-" + t }
func Network(t string) string       { return "n-" + t }
func Instance(w string) string      { return "w-" + w }
func HomeVolume(w string) string    { return "h-" + w }
func Dataset(root, t string) string { return root + "/" + t }
