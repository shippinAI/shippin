package api

import (
	"net"
	"strings"

	"shippin.cloud/console/internal/goldsign"
)

// goldSignStore builds the sign store from env only. CONSOLE_PUBLIC_HOST (e.g. the public console
// host) is the WebAuthn RP ID and gives the https origin; GOLDSIGN_RP_ID / GOLDSIGN_ORIGINS override
// it. GOLDSIGN_OWNER_NAME is the approver name Gold expects (GOLD_APPROVER there). Returns nil when
// anything required is missing, which leaves the feature off.
func goldSignStore(getenv func(string) string) *goldsign.Store {
	host := strings.TrimSpace(getenv("CONSOLE_PUBLIC_HOST"))
	if host != "" && (strings.ContainsAny(host, "/:@ ") || net.ParseIP(host) != nil) {
		return nil // a bare DNS name only: RP IDs cannot carry a scheme, port or path
	}
	rp := firstNonEmpty(getenv("GOLDSIGN_RP_ID"), host)
	origins := strings.Fields(getenv("GOLDSIGN_ORIGINS"))
	if len(origins) == 0 && host != "" {
		origins = []string{"https://" + host}
	}
	s := &goldsign.Store{Dir: getenv("GOLDSIGN_DIR"), FactsDir: getenv("GOLDSIGN_FACTS_DIR"),
		OwnerSub: getenv("GOLDSIGN_OWNER_SUB"), OwnerName: getenv("GOLDSIGN_OWNER_NAME"), RPID: rp, Origins: origins}
	if s.Dir == "" || s.FactsDir == "" || s.OwnerSub == "" || s.OwnerName == "" || s.RPID == "" || len(s.Origins) == 0 {
		return nil
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}
