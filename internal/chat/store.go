// Package chat backs the console chat: sessions and messages in Postgres (023_chat.sql), the
// A2UI catalog that structured replies are validated against, and parsers for the vendor CLIs'
// streamed output.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

type Session struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	ProjectID   *string   `json:"project_id,omitempty"`
	WorkspaceID string    `json:"workspace_id"`
	Owner       string    `json:"-"`
	Provider    string    `json:"provider"`
	Title       string    `json:"title"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Message struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	UI        json.RawMessage `json:"ui"`
	CreatedAt time.Time       `json:"created_at"`
}

const sesCols = `id, tenant_id, project_id, workspace_id, owner_sub, provider, title, created_at, updated_at`

func scanSession(r pgx.Row) (*Session, error) {
	var s Session
	err := r.Scan(&s.ID, &s.TenantID, &s.ProjectID, &s.WorkspaceID, &s.Owner, &s.Provider, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	return &s, err
}

// Create opens a session owned by sub.
func (s *Store) Create(ctx context.Context, x Session) (*Session, error) {
	return scanSession(s.Pool.QueryRow(ctx, `INSERT INTO chat_sessions (id, tenant_id, project_id, workspace_id, owner_sub, provider)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+sesCols, uuid.NewString(), x.TenantID, x.ProjectID, x.WorkspaceID, x.Owner, x.Provider))
}

// Get returns sub's session (nil when missing or not theirs).
func (s *Store) Get(ctx context.Context, sub, id string) (*Session, error) {
	if uuid.Validate(id) != nil {
		return nil, nil
	}
	x, err := scanSession(s.Pool.QueryRow(ctx, `SELECT `+sesCols+` FROM chat_sessions WHERE id=$1 AND owner_sub=$2`, id, sub))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return x, err
}

// List returns sub's sessions, newest activity first.
func (s *Store) List(ctx context.Context, sub string) ([]Session, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+sesCols+` FROM chat_sessions WHERE owner_sub=$1 ORDER BY updated_at DESC LIMIT 100`, sub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// Messages returns a session's messages, oldest first.
func (s *Store) Messages(ctx context.Context, id string) ([]Message, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, role, content, ui, created_at FROM chat_messages WHERE session_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.UI, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Add appends a message and touches the session (the first user message becomes the title).
func (s *Store) Add(ctx context.Context, session, role, content string, ui []Block) (*Message, error) {
	if ui == nil {
		ui = []Block{}
	}
	raw, _ := json.Marshal(ui)
	m := Message{ID: uuid.NewString(), Role: role, Content: content, UI: raw}
	err := s.Pool.QueryRow(ctx, `INSERT INTO chat_messages (id, session_id, role, content, ui) VALUES ($1,$2,$3,$4,$5) RETURNING created_at`,
		m.ID, session, role, content, raw).Scan(&m.CreatedAt)
	if err != nil {
		return nil, err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE chat_sessions SET updated_at=now(),
		title = CASE WHEN title='New chat' AND $2='user' THEN left($3, 80) ELSE title END WHERE id=$1`, session, role, content)
	return &m, err
}
