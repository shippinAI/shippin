package identity

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"
)

func TestPKCEAndWebhook(t *testing.T) {
	v := "verifier-value"
	ch := PKCEChallenge(v)
	if !PKCEMatch(v, ch) || PKCEMatch("other", ch) || PKCEMatch("", ch) {
		t.Fatal("pkce")
	}
	body := []byte(`{"event":"user.signup"}`)
	secret := []byte("signing-secret")
	now := time.Unix(1_700_000_000, 0)
	h := SignWebhook(secret, body, now)
	if !VerifyWebhook(secret, body, h, now, time.Minute) {
		t.Fatal("signature rejected")
	}
	if VerifyWebhook(secret, body, h, now.Add(time.Hour), time.Minute) {
		t.Fatal("stale signature accepted")
	}
	if VerifyWebhook([]byte("other"), body, h, now, time.Minute) {
		t.Fatal("wrong secret accepted")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	now := time.Unix(100, 0)
	if !l.Allow("a", now) || !l.Allow("a", now) || l.Allow("a", now) {
		t.Fatal("limit")
	}
	if !l.Allow("a", now.Add(time.Minute+time.Second)) {
		t.Fatal("window did not move")
	}
}

func TestPasskeyRoundTrip(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := elliptic.Marshal(elliptic.P256(), priv.X, priv.Y)
	cred := []byte("credential-id")
	challenge := bytes.Repeat([]byte{7}, 16)
	origin, rp := "https://console.example.invalid", "console.example.invalid"
	cose := appendCOSE(pub)
	ad := make([]byte, 37+16+2+len(cred)+len(cose))
	sum := sha256.Sum256([]byte(rp))
	copy(ad, sum[:])
	ad[32] = 0x41
	binary.BigEndian.PutUint16(ad[37+16:], uint16(len(cred)))
	copy(ad[37+18:], cred)
	copy(ad[37+18+len(cred):], cose)
	client := []byte(`{"type":"webauthn.create","challenge":"` + b64(challenge) + `","origin":"` + origin + `"}`)
	key, err := ParseRegistration(Registration{ClientDataJSON: client, AuthenticatorData: ad}, challenge, origin, rp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key.CredentialID, cred) || !bytes.Equal(key.PublicKey, pub) {
		t.Fatal("stored key")
	}
	authData := make([]byte, 37)
	copy(authData, sum[:])
	authData[32] = 0x01
	binary.BigEndian.PutUint32(authData[33:], 1)
	loginClient := []byte(`{"type":"webauthn.get","challenge":"` + b64(challenge) + `","origin":"` + origin + `"}`)
	clientHash := sha256.Sum256(loginClient)
	msg := append(append([]byte{}, authData...), clientHash[:]...)
	sig, err := SignES256(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	count, err := VerifyAssertion(key, Assertion{CredentialID: cred, ClientDataJSON: loginClient, AuthenticatorData: authData, Signature: sig}, challenge, origin, rp)
	if err != nil || count != 1 {
		t.Fatal(err, count)
	}
	if _, err := VerifyAssertion(key, Assertion{CredentialID: cred, ClientDataJSON: loginClient, AuthenticatorData: authData, Signature: sig}, []byte("nope"), origin, rp); err == nil {
		t.Fatal("wrong challenge accepted")
	}
}
