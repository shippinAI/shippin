package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// mountIdentity answers the SPA's session endpoints from the demo identity file
// ({"me": {...}, "access": {...}}), so the admin shell renders without an IdP.
func mountIdentity(mux *http.ServeMux, raw []byte) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		log.Fatalf("identity: %v", err)
	}
	serve := func(key string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(doc[key])
		}
	}
	mux.HandleFunc("GET /v1/me", serve("me"))
	mux.HandleFunc("GET /v1/operator/access", serve("access"))
	mux.HandleFunc("GET /v1/settings/public", serve("public"))
	mux.HandleFunc("GET /v1/operator/break-glass/pending", serve("empty"))
	mux.HandleFunc("GET /v1/operator/access-requests", serve("empty"))
}
