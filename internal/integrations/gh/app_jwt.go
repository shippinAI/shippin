package gh

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

// JWT is the App's RS256 assertion: iat 60 s in the past (clock drift), exp 9 min ahead (GitHub's
// limit is 10), iss the App's client id.
func (a *App) JWT() (string, error) {
	now := a.now().Unix()
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iat": now - 60, "exp": now + 9*60, "iss": a.ClientID})
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString(head) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc.EncodeToString(sig), nil
}

func (a *App) appAuth() (string, error) {
	j, err := a.JWT()
	return "Bearer " + j, err
}
