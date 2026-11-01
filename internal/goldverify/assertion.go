package goldverify

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
)

// Envelope is what the console relays. It carries no secret; an agent may copy or inject one, but
// cannot make one verify without the operator's passkey.
type Envelope struct {
	Payload           Payload `json:"payload"`
	KID               string  `json:"kid"`
	CredentialID      string  `json:"credential_id"`
	AuthenticatorData string  `json:"authenticator_data"` // base64url
	ClientDataJSON    string  `json:"client_data_json"`   // base64url
	Signature         string  `json:"signature"`          // base64url
}

type clientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

const (
	flagUP = 0x01
	flagUV = 0x04
)

// checkAssertion is the WebAuthn §7.2 subset needed offline: type, challenge, origin, rpIdHash,
// UP+UV flags and the signature over authenticatorData || sha256(clientDataJSON).
func checkAssertion(e *Envelope, k *PinnedKey) error {
	if e.CredentialID != k.CredentialID {
		return errors.New("credential id does not match pinned key")
	}
	ad, err1 := B64.DecodeString(e.AuthenticatorData)
	cdj, err2 := B64.DecodeString(e.ClientDataJSON)
	sig, err3 := B64.DecodeString(e.Signature)
	if err := errors.Join(err1, err2, err3); err != nil {
		return errors.New("bad base64 in envelope")
	}
	var cd clientData
	if err := json.Unmarshal(cdj, &cd); err != nil {
		return errors.New("bad clientDataJSON")
	}
	if cd.Type != "webauthn.get" {
		return errors.New("clientData type is not webauthn.get")
	}
	if cd.Challenge != B64.EncodeToString(e.Payload.Challenge()) {
		return errors.New("challenge does not match payload")
	}
	if cd.CrossOrigin || !slices.Contains(k.Origins, cd.Origin) {
		return errors.New("origin not allowed")
	}
	if len(ad) < 37 {
		return errors.New("authenticatorData too short")
	}
	rp := sha256.Sum256([]byte(k.RPID))
	if !bytes.Equal(ad[:32], rp[:]) {
		return errors.New("rpIdHash mismatch")
	}
	if ad[32]&flagUP == 0 || ad[32]&flagUV == 0 {
		return errors.New("user presence and verification required")
	}
	ch := sha256.Sum256(cdj)
	signed := append(append([]byte{}, ad...), ch[:]...)
	switch pub := k.pub.(type) {
	case *ecdsa.PublicKey:
		d := sha256.Sum256(signed)
		if !ecdsa.VerifyASN1(pub, d[:], sig) {
			return errors.New("bad signature")
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(pub, signed, sig) {
			return errors.New("bad signature")
		}
	default:
		return errors.New("unsupported key type")
	}
	return nil
}
