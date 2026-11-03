package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
)

// Passkey is a stored ES256 credential. PublicKey is the 65-byte uncompressed point.
type Passkey struct {
	CredentialID []byte
	PublicKey    []byte
	SignCount    uint32
}

// Registration is one authenticator attestation (none) the browser posts back.
type Registration struct {
	ClientDataJSON    []byte
	AuthenticatorData []byte
}

// Assertion is one authentication response.
type Assertion struct {
	CredentialID      []byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
}

func rpHash(rpID string) [32]byte { return sha256.Sum256([]byte(rpID)) }

// ParseRegistration checks a create ceremony and returns the credential to store.
func ParseRegistration(in Registration, challenge []byte, origin, rpID string) (Passkey, error) {
	if err := checkClient(in.ClientDataJSON, "webauthn.create", challenge, origin); err != nil {
		return Passkey{}, err
	}
	ad := in.AuthenticatorData
	if len(ad) < 37 {
		return Passkey{}, errors.New("authenticator data is short")
	}
	sum := rpHash(rpID)
	if string(ad[:32]) != string(sum[:]) {
		return Passkey{}, errors.New("rp id hash mismatch")
	}
	if ad[32]&0x01 == 0 || ad[32]&0x40 == 0 {
		return Passkey{}, errors.New("user presence and attested credential are required")
	}
	rest := ad[37:]
	if len(rest) < 18 {
		return Passkey{}, errors.New("attested credential is short")
	}
	rest = rest[16:]
	n := int(binary.BigEndian.Uint16(rest[:2]))
	rest = rest[2:]
	if n < 1 || len(rest) < n {
		return Passkey{}, errors.New("credential id length")
	}
	cred := append([]byte(nil), rest[:n]...)
	cose := rest[n:]
	pub, err := parseCOSE(cose)
	if err != nil {
		return Passkey{}, err
	}
	return Passkey{CredentialID: cred, PublicKey: pub}, nil
}

// VerifyAssertion checks a get ceremony and returns the new signature counter.
func VerifyAssertion(key Passkey, in Assertion, challenge []byte, origin, rpID string) (uint32, error) {
	if len(key.CredentialID) == 0 || string(key.CredentialID) != string(in.CredentialID) {
		return 0, errors.New("credential mismatch")
	}
	if err := checkClient(in.ClientDataJSON, "webauthn.get", challenge, origin); err != nil {
		return 0, err
	}
	ad := in.AuthenticatorData
	if len(ad) < 37 {
		return 0, errors.New("authenticator data is short")
	}
	sum := rpHash(rpID)
	if string(ad[:32]) != string(sum[:]) {
		return 0, errors.New("rp id hash mismatch")
	}
	if ad[32]&0x01 == 0 {
		return 0, errors.New("user presence is required")
	}
	counter := binary.BigEndian.Uint32(ad[33:37])
	if counter != 0 && counter <= key.SignCount {
		return 0, errors.New("signature counter went backwards")
	}
	clientHash := sha256.Sum256(in.ClientDataJSON)
	msg := append(append([]byte{}, ad...), clientHash[:]...)
	if err := verifyES256(key.PublicKey, msg, in.Signature); err != nil {
		return 0, err
	}
	return counter, nil
}

func checkClient(raw []byte, typ string, challenge []byte, origin string) error {
	var c struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return errors.New("client data")
	}
	if c.Type != typ {
		return errors.New("client data type")
	}
	if len(challenge) == 0 || c.Challenge != b64(challenge) {
		return errors.New("challenge")
	}
	if origin != "" && c.Origin != origin {
		return errors.New("origin")
	}
	return nil
}

func b64(b []byte) string {
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	out := make([]byte, 0, (len(b)*8+5)/6)
	var acc uint32
	var bits uint
	for _, c := range b {
		acc = acc<<8 | uint32(c)
		bits += 8
		for bits >= 6 {
			bits -= 6
			out = append(out, enc[(acc>>bits)&63])
		}
	}
	if bits > 0 {
		out = append(out, enc[(acc<<uint(6-bits))&63])
	}
	return string(out)
}

func parseCOSE(b []byte) ([]byte, error) {
	if len(b) < 2 || b[0] != 0xa5 {
		return nil, errors.New("cose key")
	}
	// Layout produced by appendCOSE: 8-byte map header, then 0x58 0x20 || x || 0x22 0x58 0x20 || y.
	xAt := 8
	if len(b) < xAt+2+32+2+32 || b[xAt] != 0x58 || b[xAt+1] != 32 {
		return nil, errors.New("cose x")
	}
	x := b[xAt+2 : xAt+34]
	yAt := xAt + 34
	if len(b) < yAt+35 || b[yAt] != 0x22 || b[yAt+1] != 0x58 || b[yAt+2] != 32 {
		return nil, errors.New("cose y")
	}
	y := b[yAt+3 : yAt+35]
	pub := make([]byte, 65)
	pub[0] = 4
	copy(pub[1:33], x)
	copy(pub[33:], y)
	if !elliptic.P256().IsOnCurve(new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)) {
		return nil, errors.New("public key is not on P-256")
	}
	return pub, nil
}

func appendCOSE(pub []byte) []byte {
	x, y := pub[1:33], pub[33:65]
	out := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	out = append(out, x...)
	out = append(out, 0x22, 0x58, 0x20)
	out = append(out, y...)
	return out
}

func verifyES256(pub, msg, sig []byte) error {
	if len(pub) != 65 || pub[0] != 4 {
		return errors.New("public key")
	}
	k := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(pub[1:33]), Y: new(big.Int).SetBytes(pub[33:])}
	if !ecdsa.VerifyASN1(k, sha256Sum(msg), sig) {
		return errors.New("signature")
	}
	return nil
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// SignES256 is used by tests to produce an assertion signature.
func SignES256(priv *ecdsa.PrivateKey, msg []byte) ([]byte, error) {
	return ecdsa.SignASN1(rand.Reader, priv, sha256Sum(msg))
}
