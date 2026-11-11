package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey struct{}

// Token returns the caller's bearer token from the session cookie or header.
func (f *Flow) Token(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if s := f.cookieSession(r); s != nil {
		return s.AccessToken
	}
	return ""
}

type localKey struct{}

// Require rejects requests without a token; the IdP validates it downstream. A local directory
// session (setup, recovery code, passkey, magic link) carries no IdP token; it passes with the
// local user id in the context instead, and the API resolves that user from its own database.
func (f *Flow) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := f.Token(r)
		if t == "" {
			if s := f.cookieSession(r); s != nil && s.UserID != "" {
				ctx := context.WithValue(r.Context(), localKey{}, s.UserID)
				next(w, r.WithContext(context.WithValue(ctx, methodKey{}, s.Method)))
				return
			}
			Problem(w, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, t)))
	}
}

type methodKey struct{}

// SignInMethod is how the current session was signed in. A bearer token from an identity provider is
// identity_provider; an older local session that did not record it is password.
func SignInMethod(ctx context.Context) string {
	if LocalUser(ctx) == "" {
		return "identity_provider"
	}
	if m, _ := ctx.Value(methodKey{}).(string); m != "" {
		return m
	}
	return "password"
}

// LocalUser is the local directory user id Require admitted, or "" for an IdP token.
func LocalUser(ctx context.Context) string {
	v, _ := ctx.Value(localKey{}).(string)
	return v
}

func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
