package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"shippin.cloud/console/internal/db"
)

// Session is server-side state; the browser only holds an opaque cookie.
type Session struct {
	AccessToken string    `json:"at,omitempty"`
	Verifier    string    `json:"v,omitempty"`
	State       string    `json:"s,omitempty"`
	ReturnTo    string    `json:"r,omitempty"`
	Expires     time.Time `json:"e"`
	// AuthAt is when the user last authenticated at the IdP (id_token auth_time); zero if unknown.
	AuthAt time.Time `json:"a,omitempty"`
	// Aud is the app the session was signed in through: "" = console, AudOperator = ops origin.
	Aud string `json:"au,omitempty"`
	// UserID is set for a local directory session. Empty means an external-issuer session.
	UserID string `json:"uid,omitempty"`
}

// Store persists sessions in Postgres, AES-256-GCM encrypted (key from SHIPPIN_SESSION_KEY).
type Store struct {
	db   *db.DB
	aead cipher.AEAD
}

func NewStore(d *db.DB, hexKey string) (*Store, error) {
	k, err := hex.DecodeString(hexKey)
	if err != nil || len(k) != 32 {
		return nil, errors.New("SHIPPIN_SESSION_KEY must be 64 hex chars")
	}
	blk, _ := aes.NewCipher(k)
	aead, _ := cipher.NewGCM(blk)
	return &Store{db: d, aead: aead}, nil
}

func (s *Store) New(ctx context.Context, sess *Session) (string, error) {
	id := RandomString(32)
	raw, _ := json.Marshal(sess)
	nonce := make([]byte, s.aead.NonceSize())
	_, _ = rand.Read(nonce)
	sealed := s.aead.Seal(nonce, nonce, raw, []byte(id))
	return id, s.db.PutSession(ctx, id, sealed, sess.Expires)
}

func (s *Store) Get(ctx context.Context, id string) *Session {
	b, err := s.db.GetSession(ctx, id)
	if err != nil || len(b) < s.aead.NonceSize() {
		return nil
	}
	n := s.aead.NonceSize()
	raw, err := s.aead.Open(nil, b[:n], b[n:], []byte(id))
	if err != nil {
		return nil
	}
	sess := &Session{}
	if json.Unmarshal(raw, sess) != nil || time.Now().After(sess.Expires) {
		return nil
	}
	return sess
}

// Console reports whether this is a console sign-in session (not a pending login, a one-time
// workspace code, or a workspace-host session, which carry State or ReturnTo).
func (s *Session) Console() bool {
	return s != nil && s.AccessToken != "" && s.State == "" && s.ReturnTo == ""
}

// Workspace reports whether this is a workspace-host session bound to host.
func (s *Session) Workspace(host string) bool {
	return s != nil && s.AccessToken != "" && s.State == "" && s.ReturnTo != "" && s.ReturnTo == host
}

func (s *Store) Delete(ctx context.Context, id string) { _ = s.db.DeleteSession(ctx, id) }

func RandomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
