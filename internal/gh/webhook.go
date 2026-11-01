package gh

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// VerifySignature checks X-Hub-Signature-256 ("sha256=<hex>") over the raw body in constant time.
func VerifySignature(secret string, body []byte, header string) bool {
	hexsig, ok := strings.CutPrefix(header, "sha256=")
	if !ok || secret == "" {
		return false
	}
	got, err := hex.DecodeString(hexsig)
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hmac.Equal(got, m.Sum(nil))
}

// Event is the subset of installation, installation_repositories and repository payloads used.
type Event struct {
	Action       string       `json:"action"`
	Installation Installation `json:"installation"`
	Removed      []Repo       `json:"repositories_removed"`
	Repository   *Repo        `json:"repository"`
}

// ParseEvent decodes a verified webhook body.
func ParseEvent(body []byte) (*Event, error) {
	e := &Event{}
	return e, json.Unmarshal(body, e)
}
