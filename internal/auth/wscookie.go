package auth

import (
	"net/http"
	"strings"
	"time"
)

// WSCookie is the per-workspace-host session cookie (host-only, never shared across hosts).
const WSCookie = "shippin_ws"

func SetWSCookie(w http.ResponseWriter, id string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{Name: WSCookie, Value: id, Path: "/", MaxAge: int(time.Until(exp).Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

// SessionToken returns the access token of the console session cookie, or of an
// Authorization: Bearer header (same as the API), if any. Callers still verify it.
func (f *Flow) SessionToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if s := f.cookieSession(r); s != nil {
		return s.AccessToken
	}
	return ""
}
