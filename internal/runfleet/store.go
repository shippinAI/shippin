package runfleet

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Runner is one row of ci_runners.json. The first fields match the reference
// fleet file so an existing file still loads. Extra fields are this control plane.
type Runner struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Kind            string   `json:"kind,omitempty"`
	VMID            int      `json:"vmid,omitempty"`
	Host            string   `json:"host,omitempty"`
	Repos           []string `json:"repos"`
	Size            string   `json:"size,omitempty"`
	Labels          []string `json:"labels,omitempty"`
	Account         string   `json:"account,omitempty"`
	AccountKind     string   `json:"account_kind,omitempty"`
	Status          string   `json:"status,omitempty"`
	ProvisionDetail string   `json:"provision_detail,omitempty"`
}

// ConnectedRepo is one repository on a GitHub account connection.
type ConnectedRepo struct {
	FullName       string `json:"full_name"`
	Private        bool   `json:"private"`
	License        string `json:"license,omitempty"`
	Lovable        bool   `json:"lovable"`
	Approval       string `json:"approval"`
	ApprovalReason string `json:"approval_reason"`
	RetentionDays  int    `json:"retention_days"`
	RunnerID       string `json:"runner_id,omitempty"`
}

// Connection is the account-level onboarding record.
type Connection struct {
	ID           string          `json:"id"`
	Account      string          `json:"account"`
	AccountKind  string          `json:"account_kind"`
	Registration string          `json:"registration"`
	Size         string          `json:"size,omitempty"`
	Repos        []ConnectedRepo `json:"repos"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Job is a stored CI job. Logs are redacted before they are written.
type Job struct {
	ID          string    `json:"id"`
	Repo        string    `json:"repo"`
	Name        string    `json:"name"`
	Conclusion  string    `json:"conclusion"`
	RunnerName  string    `json:"runner_name,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Log         string    `json:"log,omitempty"`
	Redactions  int       `json:"redactions"`
	LogStored   bool      `json:"log_stored"`
	HTMLURL     string    `json:"html_url,omitempty"`
	GitHubJobID int64     `json:"github_job_id,omitempty"`
}

// JobSummary is a job without its log body.
type JobSummary struct {
	ID          string    `json:"id"`
	Repo        string    `json:"repo"`
	Name        string    `json:"name"`
	Conclusion  string    `json:"conclusion"`
	RunnerName  string    `json:"runner_name,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Redactions  int       `json:"redactions"`
	LogStored   bool      `json:"log_stored"`
	HTMLURL     string    `json:"html_url,omitempty"`
	GitHubJobID int64     `json:"github_job_id,omitempty"`
}

func (j Job) Summary() JobSummary {
	return JobSummary{
		ID: j.ID, Repo: j.Repo, Name: j.Name, Conclusion: j.Conclusion,
		RunnerName: j.RunnerName, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
		Redactions: j.Redactions, LogStored: j.LogStored, HTMLURL: j.HTMLURL,
		GitHubJobID: j.GitHubJobID,
	}
}

// Installation is a GitHub App installation recorded from a signed webhook.
type Installation struct {
	ID          int64  `json:"id"`
	Account     string `json:"account"`
	AccountKind string `json:"account_kind"`
}

type disk struct {
	Runners       []Runner       `json:"runners"`
	Connections   []Connection   `json:"connections"`
	Jobs          []Job          `json:"jobs"`
	Installations []Installation `json:"installations"`
}

func loadDisk(dir string) (*disk, error) {
	d := &disk{}
	runnersPath := filepath.Join(dir, "ci_runners.json")
	if b, err := os.ReadFile(runnersPath); err == nil {
		var file struct {
			Runners []Runner `json:"runners"`
		}
		if err := json.Unmarshal(b, &file); err != nil {
			return nil, fmt.Errorf("ci_runners.json: %w", err)
		}
		seen := map[string]bool{}
		for _, r := range file.Runners {
			if r.ID == "" || r.Name == "" {
				return nil, fmt.Errorf("ci_runners.json: a runner is missing id or name")
			}
			if seen[r.ID] {
				return nil, fmt.Errorf("ci_runners.json: duplicate runner id %s", r.ID)
			}
			seen[r.ID] = true
			if r.Repos == nil {
				r.Repos = []string{}
			}
			d.Runners = append(d.Runners, r)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	statePath := filepath.Join(dir, "state.json")
	if b, err := os.ReadFile(statePath); err == nil {
		var rest struct {
			Connections   []Connection   `json:"connections"`
			Jobs          []Job          `json:"jobs"`
			Installations []Installation `json:"installations"`
		}
		if err := json.Unmarshal(b, &rest); err != nil {
			return nil, fmt.Errorf("state.json: %w", err)
		}
		d.Connections = rest.Connections
		d.Jobs = rest.Jobs
		d.Installations = rest.Installations
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return d, nil
}

func saveDisk(dir string, d *disk) error {
	if d.Runners == nil {
		d.Runners = []Runner{}
	}
	if err := writeJSON(filepath.Join(dir, "ci_runners.json"), map[string]any{"runners": d.Runners}); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "state.json"), map[string]any{
		"connections":   nonempty(d.Connections),
		"jobs":          nonemptyJobs(d.Jobs),
		"installations": nonemptyInst(d.Installations),
	})
}

func nonempty(v []Connection) []Connection {
	if v == nil {
		return []Connection{}
	}
	return v
}
func nonemptyJobs(v []Job) []Job {
	if v == nil {
		return []Job{}
	}
	return v
}
func nonemptyInst(v []Installation) []Installation {
	if v == nil {
		return []Installation{}
	}
	return v
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
