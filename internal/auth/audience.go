package auth

import (
	"net"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

// AudOperator marks sessions signed in through the operator app on its own origin (ops.<domain>).
const AudOperator = "operator"

// Audience is the app a request belongs to, by its Host: AudOperator on the configured operator
// host, "" (the console) everywhere else.
func (f *Flow) Audience(r *http.Request) string {
	if h := f.cfg.Operator.Host; h != "" && strings.EqualFold(hostOnly(r.Host), h) {
		return AudOperator
	}
	return ""
}

// IsOperatorOrigin reports whether r arrived on the operator app's own origin.
func (f *Flow) IsOperatorOrigin(r *http.Request) bool { return f.Audience(r) == AudOperator }

func (f *Flow) app(r *http.Request) (string, *oauth2.Config) {
	if aud := f.Audience(r); aud == AudOperator {
		return aud, &f.ops
	}
	return "", &f.oa
}

// cookieSession is the signed-in session of r's cookie, only if it was made by the same app as
// r's origin: a console session is never accepted on the operator origin, nor the reverse.
func (f *Flow) cookieSession(r *http.Request) *Session {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	s := f.Store.Get(r.Context(), c.Value)
	if s != nil && s.Aud == f.Audience(r) && (s.Console() || s.UserID != "") {
		return s
	}
	return nil
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}
