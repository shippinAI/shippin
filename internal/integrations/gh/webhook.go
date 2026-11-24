package gh

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
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

// Event is the subset of installation, installation_repositories, repository and workflow_job
// payloads used.
type Event struct {
	Action       string       `json:"action"`
	Installation Installation `json:"installation"`
	Removed      []Repo       `json:"repositories_removed"`
	Repository   *Repo        `json:"repository"`
	WorkflowJob  *WorkflowJob `json:"workflow_job"`
}

// WorkflowJob is a workflow_job payload's job: which run it is in, the runs-on labels it asked for,
// and, as it moves on, the runner that took it and how it ended.
type WorkflowJob struct {
	ID          int64      `json:"id"`
	RunID       int64      `json:"run_id"`
	Name        string     `json:"name"`
	Labels      []string   `json:"labels"`
	RunnerName  string     `json:"runner_name"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// ParseEvent decodes a verified webhook body.
func ParseEvent(body []byte) (*Event, error) {
	e := &Event{}
	return e, json.Unmarshal(body, e)
}
