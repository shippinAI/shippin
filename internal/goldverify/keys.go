package goldverify

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// PinnedKey is one of the operator's passkeys, pinned on Gold by a reviewed T2 ticket. Public data only.
type PinnedKey struct {
	KID          string   `json:"kid"`
	Owner        string   `json:"owner"` // must equal the approver user (operator)
	CredentialID string   `json:"credential_id"`
	SPKI         string   `json:"spki"` // base64url DER SubjectPublicKeyInfo (from getPublicKey())
	Alg          int      `json:"alg"`  // -7 ES256, -8 EdDSA
	RPID         string   `json:"rp_id"`
	Origins      []string `json:"origins"`
	pub          crypto.PublicKey
}

// Fingerprint is sha256 of the SPKI DER, shown in the console so the operator can compare before pinning.
func Fingerprint(spki []byte) string {
	h := sha256.Sum256(spki)
	return hex.EncodeToString(h[:])
}

// LoadKeys reads a JSON array of pinned keys. The file must be root-owned and not group/world
// writable on Gold (checked by the caller via CheckFilePerms).
func LoadKeys(path string) ([]PinnedKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ks []PinnedKey
	if err := json.Unmarshal(b, &ks); err != nil {
		return nil, fmt.Errorf("keys: %w", err)
	}
	for i := range ks {
		if err := ks[i].parse(); err != nil {
			return nil, fmt.Errorf("key %s: %w", ks[i].KID, err)
		}
	}
	return ks, nil
}

func (k *PinnedKey) parse() error {
	der, err := B64.DecodeString(k.SPKI)
	if err != nil {
		return err
	}
	k.pub, err = x509.ParsePKIXPublicKey(der)
	if err != nil {
		return err
	}
	if k.Alg != -7 && k.Alg != -8 {
		return errors.New("unsupported alg")
	}
	if k.RPID == "" || len(k.Origins) == 0 {
		return errors.New("rp_id and origins required")
	}
	return nil
}

// find returns the pinned key for kid, but only if it belongs to owner.
func find(ks []PinnedKey, kid, owner string) (*PinnedKey, error) {
	for i := range ks {
		if ks[i].KID == kid {
			if ks[i].Owner != owner {
				return nil, fmt.Errorf("key %s belongs to %q, not %q", kid, ks[i].Owner, owner)
			}
			return &ks[i], nil
		}
	}
	return nil, fmt.Errorf("key %s is not pinned", kid)
}
