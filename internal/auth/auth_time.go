package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// authTime reads auth_time from the id_token returned by the token endpoint. The token came
// straight from the IdP over TLS in the code exchange, so its payload is trusted as-is; a
// missing or malformed claim yields zero (treated as "not recently authenticated").
func authTime(tok *oauth2.Token) time.Time {
	raw, _ := tok.Extra("id_token").(string)
	return idTokenAuthTime(raw)
}

func idTokenAuthTime(raw string) time.Time {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		AuthTime int64 `json:"auth_time"`
	}
	if json.Unmarshal(body, &claims) != nil || claims.AuthTime <= 0 {
		return time.Time{}
	}
	return time.Unix(claims.AuthTime, 0)
}

// AuthTime is when the caller of r last signed in, at the IdP or locally. Only console cookie sessions
// carry it; bearer tokens (PATs) return zero and can never pass a re-authentication check.
func (f *Flow) AuthTime(r *http.Request) time.Time {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return time.Time{}
	}
	if s := f.cookieSession(r); s != nil {
		return s.AuthAt
	}
	return time.Time{}
}
