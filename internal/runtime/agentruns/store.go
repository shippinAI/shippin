// Package agentruns stores M4 build-agent runs (019_prompt.sql). A run records which of the user's
// own CLIs builds a project, in which workspace, and how far it got. No token is ever stored, and
// the prompt text is kept only until it has been written into the workspace.
package agentruns

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// States (019_prompt.sql).
const (
	Queued       = "queued"
	Provisioning = "provisioning"
	WaitingAI    = "waiting_ai"
	Starting     = "starting"
	Running      = "running"
	Exited       = "exited"
	Failed       = "failed"
)

var ErrNotFound = errors.New("agent run not found")

// Run is the user-facing view of one run (no prompt text, no hash).
type Run struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	ProjectID   string     `json:"project_id"`
	WorkspaceID string     `json:"workspace_id,omitempty"`
	AccountID   string     `json:"-"`
	CreatedBy   string     `json:"-"`
	Provider    string     `json:"provider"`
	Template    string     `json:"template"`
	Preview     string     `json:"prompt_preview"`
	State       string     `json:"state"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Store struct{ Pool *pgxpool.Pool }

const cols = `id::text, tenant_id::text, project_id::text, coalesce(workspace_id::text,''), account_id::text, created_by,
  provider, template, prompt_preview, state, coalesce(error,''), started_at, ended_at, created_at`

func scan(row pgx.Row) (*Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.TenantID, &r.ProjectID, &r.WorkspaceID, &r.AccountID, &r.CreatedBy, &r.Provider, &r.Template,
		&r.Preview, &r.State, &r.Error, &r.StartedAt, &r.EndedAt, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

// Preview is the first 200 bytes of the prompt, cut on a rune boundary.
func Preview(prompt string) string {
	if len(prompt) <= 200 {
		return prompt
	}
	p := prompt[:200]
	for !utf8.ValidString(p) {
		p = p[:len(p)-1]
	}
	return p
}

// Create records a queued run holding the prompt until it reaches the workspace.
func (s *Store) Create(ctx context.Context, r Run, prompt string) (*Run, error) {
	id, _ := uuid.NewV7()
	sum := sha256.Sum256([]byte(prompt))
	return scan(s.Pool.QueryRow(ctx, `INSERT INTO agent_runs(id,tenant_id,project_id,account_id,created_by,provider,template,
	    prompt_sha256,prompt_preview,prompt_pending) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+cols,
		id, r.TenantID, r.ProjectID, r.AccountID, r.CreatedBy, r.Provider, r.Template, sum[:], Preview(prompt), prompt))
}

// Get returns one run of tenant.
func (s *Store) Get(ctx context.Context, tenant, id string) (*Run, error) {
	return scan(s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM agent_runs WHERE tenant_id=$1 AND id::text=$2`, tenant, id))
}

// Latest returns the newest run of a project.
func (s *Store) Latest(ctx context.Context, tenant, project string) (*Run, error) {
	return scan(s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM agent_runs WHERE tenant_id=$1 AND project_id::text=$2
	  ORDER BY created_at DESC LIMIT 1`, tenant, project))
}

// Pending returns the prompt still waiting to be written ("" once delivered).
func (s *Store) Pending(ctx context.Context, id string) (string, error) {
	var p *string
	err := s.Pool.QueryRow(ctx, `SELECT prompt_pending FROM agent_runs WHERE id::text=$1`, id).Scan(&p)
	if p == nil {
		return "", err
	}
	return *p, err
}

// Delivered drops the prompt text once hostd has written it into the workspace.
func (s *Store) Delivered(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE agent_runs SET prompt_pending=NULL, updated_at=now() WHERE id::text=$1`, id)
	return err
}

// SetWorkspace links the run to the workspace it builds in.
func (s *Store) SetWorkspace(ctx context.Context, id, ws string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE agent_runs SET workspace_id=$2, updated_at=now() WHERE id::text=$1`, id, ws)
	return err
}

// SetState moves a run; running stamps started_at, exited/failed stamp ended_at.
func (s *Store) SetState(ctx context.Context, id, state, msg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE agent_runs SET state=$2, error=NULLIF($3,''), updated_at=now(),
	    started_at=CASE WHEN $2='running' THEN coalesce(started_at, now()) ELSE started_at END,
	    ended_at=CASE WHEN $2 IN ('exited','failed') THEN now() WHEN $2='running' THEN NULL ELSE ended_at END
	  WHERE id::text=$1`, id, state, msg)
	return err
}

// Waiting lists an account's runs in tenant parked until their AI sign-in is connected.
func (s *Store) Waiting(ctx context.Context, account, tenant string) ([]Run, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+cols+` FROM agent_runs WHERE account_id::text=$1 AND tenant_id::text=$2
	  AND state='waiting_ai' ORDER BY created_at`, account, tenant)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Run, error) {
		x, err := scan(r)
		if err != nil {
			return Run{}, err
		}
		return *x, nil
	})
}
