package auth

import (
	"net/http"
	"time"
)

// Current is the decrypted session cookie, or nil.
func (f *Flow) Current(r *http.Request) *Session { return f.cookieSession(r) }

// StartLocal stores a directory session and sets the existing session cookie.
func (f *Flow) StartLocal(w http.ResponseWriter, r *http.Request, userID string) (string, error) {
	id, err := f.Store.New(r.Context(), &Session{UserID: userID, Expires: time.Now().Add(12 * time.Hour)})
	if err != nil {
		return "", err
	}
	setCookie(w, cookieName, id, 12*60*60)
	return id, nil
}
