package auth

import (
	"net/http"
	"time"
)

// Current is the decrypted session cookie, or nil.
func (f *Flow) Current(r *http.Request) *Session { return f.cookieSession(r) }

// StartLocal stores a directory session and sets the existing session cookie. Every caller has just
// proved who the user is (setup code, magic link, passkey or recovery code), so it counts as a
// sign-in for the checks that ask how recent one is.
func (f *Flow) StartLocal(w http.ResponseWriter, r *http.Request, userID, method string) (string, error) {
	id, err := f.Store.New(r.Context(), &Session{UserID: userID, Method: method, AuthAt: time.Now(), Expires: time.Now().Add(12 * time.Hour)})
	if err != nil {
		return "", err
	}
	setCookie(w, cookieName, id, 12*60*60)
	return id, nil
}
