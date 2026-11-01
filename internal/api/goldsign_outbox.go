package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/goldsign"
)

// gold admits Gold's root sync unit: the sync bearer (GOLDSIGN_SYNC_TOKEN; it can read envelopes
// and replace synced facts, never sign or approve) AND a source address inside GOLDSIGN_SYNC_CIDRS.
// Fail-closed: with no CIDRs configured every sync request is refused.
func (h *goldSignHTTP) gold(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if h.syncToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(h.syncToken)) != 1 || !h.fromSyncNet(r) {
			auth.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		next(w, r)
	}
}

func (h *goldSignHTTP) fromSyncNet(r *http.Request) bool {
	if len(h.syncNets) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil {
		return false
	}
	for _, n := range h.syncNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// parseNets is strict: any entry that is not a CIDR is an error, so a typo can never silently
// turn the source check off. An empty value means no sync source is allowed (sync refused; the caller logs that).
func parseNets(s string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, f := range strings.Fields(s) {
		_, n, err := net.ParseCIDR(f)
		if err != nil {
			return nil, fmt.Errorf("GOLDSIGN_SYNC_CIDRS: %q is not a CIDR", f)
		}
		out = append(out, n)
	}
	return out, nil
}

func (h *goldSignHTTP) outbox(w http.ResponseWriter, r *http.Request) {
	items, _ := h.s.Outbox()
	writeJSON(w, items)
}

func (h *goldSignHTTP) sync(w http.ResponseWriter, r *http.Request) {
	var in goldsign.Sync
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid sync body.")
		return
	}
	if err := h.s.ApplySync(&in); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]int{"facts": len(in.Facts), "certs": len(in.Certs), "revoked": len(in.Revoked)})
}

type credRef struct {
	CredentialID string `json:"credential_id"`
	KID          string `json:"kid"`
}

// credIDs is the allowCredentials/excludeCredentials list (public ids only).
func credIDs(ks []goldsign.KeyView) []credRef {
	out := []credRef{}
	for _, k := range ks {
		out = append(out, credRef{k.CredentialID, k.KID})
	}
	return out
}
