package main

import (
	"net/http"
	"strings"
)

// sameOrigin is api.SameOrigin's rule with the port kept: the demo runs on a loopback port, while
// the console's check assumes the edge's :443. Cross-origin writes are refused.
func sameOrigin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		site, o := r.Header.Get("Sec-Fetch-Site"), r.Header.Get("Origin")
		if write && ((site != "" && site != "same-origin" && site != "none") || (o != "" && !strings.EqualFold(o, "https://"+r.Host))) {
			http.Error(w, `{"title":"Cross-origin request refused.","status":403}`, http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}
