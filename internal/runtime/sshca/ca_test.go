package sshca

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSignBindsOnePrincipalAndExpires(t *testing.T) {
	_, caKey, _ := ed25519.GenerateKey(rand.Reader)
	s, _ := ssh.NewSignerFromKey(caKey)
	userPub, _, _ := ed25519.GenerateKey(rand.Reader)
	up, _ := ssh.NewPublicKey(userPub)
	line, until, err := New(s).Sign(Request{PublicKey: string(ssh.MarshalAuthorizedKey(up)), Principal: "w-1", KeyID: "acct", TTL: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	c := k.(*ssh.Certificate)
	if len(c.ValidPrincipals) != 1 || c.ValidPrincipals[0] != "w-1" || c.KeyId != "acct" {
		t.Fatalf("cert %+v", c)
	}
	if time.Until(until) > 6*time.Minute {
		t.Fatal("ttl not applied")
	}
	chk := ssh.CertChecker{IsUserAuthority: func(a ssh.PublicKey) bool { return string(a.Marshal()) == string(s.PublicKey().Marshal()) }}
	if err := chk.CheckCert("w-1", c); err != nil {
		t.Fatal(err)
	}
	if chk.CheckCert("w-2", c) == nil {
		t.Fatal("cert opened another workspace")
	}
	if _, _, err := New(s).Sign(Request{PublicKey: "nope", Principal: "w", TTL: time.Minute}); err == nil {
		t.Fatal("bad key accepted")
	}
}
