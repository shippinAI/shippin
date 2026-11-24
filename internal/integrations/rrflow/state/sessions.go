package state

import (
	"time"

	"github.com/google/uuid"
)

// StartSession records a new agent session for workspace.
func (s *Store) StartSession(workspace, agent, mode string) (*Session, error) {
	if !validWord(agent) || (mode != "" && !validWord(mode)) {
		return nil, ErrInvalid
	}
	id, _ := uuid.NewV7()
	x := Session{ID: id.String(), OrgID: s.org, Workspace: workspace, Agent: agent, Mode: mode, Status: "running",
		StartedAt: time.Now().UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.appendLine(sessionsFile, x); err != nil {
		return nil, err
	}
	s.sessions[x.ID], s.order = &x, append(s.order, x.ID)
	return &x, nil
}

// EndSession closes a session of workspace with a status word.
func (s *Store) EndSession(workspace, id, status string) (*Session, error) {
	if !validWord(status) {
		return nil, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.sessions[id]
	if !ok || cur.Workspace != workspace {
		return nil, ErrNotFound
	}
	x := *cur
	now := time.Now().UTC()
	x.Status, x.EndedAt = status, &now
	if err := s.appendLine(sessionsFile, x); err != nil {
		return nil, err
	}
	s.sessions[id] = &x
	return &x, nil
}

// Sessions lists the newest sessions first, filtered to workspace unless it is empty.
func (s *Store) Sessions(workspace string, limit int) []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Session{}
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		x := s.sessions[s.order[i]]
		if workspace == "" || x.Workspace == workspace {
			out = append(out, *x)
		}
	}
	return out
}
