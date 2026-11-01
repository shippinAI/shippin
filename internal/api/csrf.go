package api

import (
	"net/http"
	"strings"
)

// SameOrigin refuses cross-origin state-changing requests (auditor S-1). Published sites and dev
// URLs are same-site with the console, so SameSite=Lax cookies alone don't stop them. Browsers
// always send Origin (and Sec-Fetch-Site) on such requests; a non-browser client (bridge CLI,
// GitHub webhook) sends neither and authenticates without the session cookie, so it passes.
func SameOrigin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !crossOriginWrite(r) {
			h.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"title":"Cross-origin request refused.","status":403}`))
	})
}

func crossOriginWrite(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" && s != "same-origin" && s != "none" {
		return true
	}
	o := r.Header.Get("Origin")
	if o == "" {
		return false
	}
	return !strings.EqualFold(o, "https://"+requestHost(r)) || r.Header.Get("Origin") == "null"
}
