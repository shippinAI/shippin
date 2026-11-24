// Package backupcov is the pure part of the imported panel's backup subsystem:
// destination shape, coverage summary, and an event ring.
//
// It does not copy files, dial S3, SMB, or Infisical, or store secret bytes.
// Those executors in third_party/warden-ct102 stay in the import.
package backupcov

import (
	"fmt"
	"strings"
	"time"
)

// Item is one path a deployment wants backed up. The catalog is configuration.
// That panel's site paths are not the default catalog.
type Item struct {
	ID       string
	Group    string
	Name     string
	Path     string
	Required bool
}

// Observation is what a probe saw. It is not assumed.
type Observation struct {
	Present    bool
	Listed     bool
	Enabled    bool
	LastStatus string
}

// Summary is the coverage headline.
type Summary struct {
	Total         int
	Present       int
	Listed        int
	Required      int
	RequiredReady int
	RequiredGaps  int
	Headline      string
}

// Summarize scores items against observations. A required item is ready only when
// it is present, listed, enabled, and the last status is success.
func Summarize(items []Item, obs map[string]Observation) Summary {
	var s Summary
	s.Total = len(items)
	for _, it := range items {
		o := obs[it.ID]
		if o.Present {
			s.Present++
		}
		if o.Listed {
			s.Listed++
		}
		if !it.Required {
			continue
		}
		s.Required++
		if o.Present && o.Listed && o.Enabled && o.LastStatus == "success" {
			s.RequiredReady++
		} else {
			s.RequiredGaps++
		}
	}
	s.Headline = fmt.Sprintf("%d of %d required paths ready", s.RequiredReady, s.Required)
	return s
}

// Dest is a backup destination. SecretRef is a sops or Vaultix reference.
// Access keys are not fields.
type Dest struct {
	Kind      string // s3, smb, infisical
	Endpoint  string
	Bucket    string
	Share     string
	SecretRef string
}

// Validate rejects inline secrets and incomplete destinations.
func (d Dest) Validate() error {
	if strings.Contains(d.SecretRef, "AKIA") || strings.Contains(d.Endpoint, "@") {
		return fmt.Errorf("backup destination must not carry a secret or userinfo")
	}
	switch d.Kind {
	case "s3":
		if !strings.HasPrefix(d.Endpoint, "https://") || d.Bucket == "" || !validRef(d.SecretRef) {
			return fmt.Errorf("s3 destination needs an https endpoint, a bucket, and a secret ref")
		}
	case "smb":
		if d.Share == "" || !validRef(d.SecretRef) {
			return fmt.Errorf("smb destination needs a share and a secret ref")
		}
	case "infisical":
		if !strings.HasPrefix(d.Endpoint, "https://") || !validRef(d.SecretRef) {
			return fmt.Errorf("infisical destination needs an https endpoint and a secret ref")
		}
	default:
		return fmt.Errorf("unknown destination kind %q", d.Kind)
	}
	return nil
}

func validRef(s string) bool {
	return strings.HasPrefix(s, "sops://") || strings.HasPrefix(s, "vaultix://")
}

// Event is one backup log line.
type Event struct {
	At      time.Time
	Level   string
	Action  string
	Message string
}

// Ring keeps the newest events, capped.
type Ring struct {
	Max   int
	Items []Event
}

// Append records an event. It does not log.
func (r *Ring) Append(ev Event) {
	if r.Max <= 0 {
		r.Max = 50
	}
	r.Items = append([]Event{ev}, r.Items...)
	if len(r.Items) > r.Max {
		r.Items = r.Items[:r.Max]
	}
}
