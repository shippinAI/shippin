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

// Require rejects requests without a token; the IdP validates it downstream.
func (f *Flow) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := f.Token(r)
		if t == "" {
			Problem(w, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, t)))
	}
}

func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
