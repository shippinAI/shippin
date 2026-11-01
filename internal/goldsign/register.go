package goldsign

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"

	"shippin.cloud/console/internal/goldverify"
)

// Registration is the browser's create() result, reduced to public data.
type Registration struct {
	Label          string `json:"label"` // e.g. "iPhone", "MacBook"
	CredentialID   string `json:"credential_id"`
	SPKI           string `json:"spki"` // AuthenticatorAttestationResponse.getPublicKey(), base64url
	Alg            int    `json:"alg"`
	ClientDataJSON string `json:"client_data_json"`
}

// KeyView is a registered key as listed in the UI, with the fingerprint to compare on Gold.
type KeyView struct {
	goldverify.PinnedKey
	Label       string `json:"label"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   int64  `json:"created_at"`
}

type challenges struct {
	mu sync.Mutex
	m  map[string]time.Time
}

var regChallenges = &challenges{m: map[string]time.Time{}}

var reLabel = regexp.MustCompile(`^[\w .'-]{1,40}$`)

// BeginRegistration returns a single-use 5-minute challenge (base64url).
func (s *Store) BeginRegistration() string {
	b := make([]byte, 32)
	rand.Read(b)
	ch := goldverify.B64.EncodeToString(b)
	regChallenges.mu.Lock()
	defer regChallenges.mu.Unlock()
	regChallenges.m[ch] = s.now().Add(5 * time.Minute)
	return ch
}

// FinishRegistration stores the public key. It is NOT trusted by Gold until the operator pins it there by
// a reviewed T2 ticket (comparing the fingerprint), so a compromised console cannot add a signer.
func (s *Store) FinishRegistration(r Registration) (*KeyView, error) {
	var cd struct{ Type, Challenge, Origin string }
	raw, err := goldverify.B64.DecodeString(r.ClientDataJSON)
	if err != nil || json.Unmarshal(raw, &cd) != nil || cd.Type != "webauthn.create" {
		return nil, errors.New("bad clientDataJSON")
	}
	regChallenges.mu.Lock()
	exp, ok := regChallenges.m[cd.Challenge]
	delete(regChallenges.m, cd.Challenge)
	regChallenges.mu.Unlock()
	if !ok || s.now().After(exp) {
		return nil, errors.New("registration challenge unknown or expired")
	}
	if !slices.Contains(s.Origins, cd.Origin) {
		return nil, errors.New("origin not allowed")
	}
	der, err := goldverify.B64.DecodeString(r.SPKI)
	if err != nil {
		return nil, errors.New("bad public key")
	}
	if _, err := x509.ParsePKIXPublicKey(der); err != nil || (r.Alg != -7 && r.Alg != -8) {
		return nil, errors.New("unsupported public key")
	}
	if !reLabel.MatchString(r.Label) || r.CredentialID == "" || len(r.CredentialID) > 512 {
		return nil, errors.New("bad label or credential id")
	}
	fp := goldverify.Fingerprint(der)
	k := KeyView{Label: r.Label, Fingerprint: fp, CreatedAt: s.now().Unix(), PinnedKey: goldverify.PinnedKey{
		KID: s.OwnerName + "-" + fp[:12], Owner: s.OwnerName, CredentialID: r.CredentialID, SPKI: r.SPKI, Alg: r.Alg,
		RPID: s.RPID, Origins: s.Origins}}
	return &k, writeJSON(filepath.Join(s.Dir, "keys", k.KID+".json"), k)
}

// Keys lists registered (public) keys; also the allowCredentials list for signing.
func (s *Store) Keys() []KeyView {
	files, _ := filepath.Glob(filepath.Join(s.Dir, "keys", "*.json"))
	out := []KeyView{}
	for _, f := range files {
		var k KeyView
		if readJSON(f, &k) == nil {
			out = append(out, k)
		}
	}
	return out
}
