// Package state is rrflow/state (SHIPPIN-CLOUD/02c-rrflow.md "Modules"): sessions, memory and the
// decision log, always on. It runs as a managed service instance (si-<uuid>) on the tenant's own
// boundary VM, so the data stays inside the tenant's isolation boundary. Storage is append-only
// JSON lines; the decision log is hash-chained (02c "decision (append-only, hash-chained)").
package state

import (
	"errors"
	"regexp"
	"time"
)

// Session is one agent session (02c ai_session). Workspace comes from the caller's binding.
type Session struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	Workspace string     `json:"workspace_id"`
	Agent     string     `json:"agent"`
	Mode      string     `json:"mode"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// MemoryItem is one remembered fact (02c memory_item), scoped per workspace in the MVP.
type MemoryItem struct {
	OrgID     string    `json:"org_id"`
	Scope     string    `json:"scope"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Version   int       `json:"version"`
	Deleted   bool      `json:"deleted,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Decision is one append-only, hash-chained decision row (02c decision). Effect follows the
// policy model: allow | deny | require_approval | redact (plus hold while an approval is pending).
type Decision struct {
	Seq       int64     `json:"seq"`
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Workspace string    `json:"workspace_id"`
	Session   string    `json:"session_id,omitempty"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Effect    string    `json:"effect"`
	Reason    string    `json:"reason,omitempty"`
	ConfigVer string    `json:"config_version,omitempty"`
	At        time.Time `json:"at"`
	Prev      string    `json:"prev"`
	Hash      string    `json:"hash"`
}

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")

	keyRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
	wordRe   = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
	effects  = map[string]bool{"allow": true, "deny": true, "require_approval": true, "redact": true, "hold": true}
	maxValue = 16 << 10
	maxText  = 1024
)

// ValidKey reports whether k can name a memory item.
func ValidKey(k string) bool { return keyRe.MatchString(k) }

func validWord(s string) bool { return wordRe.MatchString(s) }
