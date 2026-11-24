package state

import (
	"sort"
	"time"
)

// PutMemory upserts key in scope (a new version line; the latest wins on load).
func (s *Store) PutMemory(scope, key, value string) (*MemoryItem, error) {
	if !ValidKey(key) || len(value) > maxValue || scope == "" {
		return nil, ErrInvalid
	}
	return s.writeMemory(scope, key, value, false)
}

// DeleteMemory tombstones key in scope.
func (s *Store) DeleteMemory(scope, key string) error {
	if _, err := s.GetMemory(scope, key); err != nil {
		return err
	}
	_, err := s.writeMemory(scope, key, "", true)
	return err
}

func (s *Store) writeMemory(scope, key, value string, del bool) (*MemoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := 1
	if cur, ok := s.memory[scope+"\x00"+key]; ok {
		v = cur.Version + 1
	}
	x := MemoryItem{OrgID: s.org, Scope: scope, Key: key, Value: value, Version: v, Deleted: del, UpdatedAt: time.Now().UTC()}
	if err := s.appendLine(memoryFile, x); err != nil {
		return nil, err
	}
	s.memory[scope+"\x00"+key] = &x
	return &x, nil
}

// GetMemory returns the live item for key in scope.
func (s *Store) GetMemory(scope, key string) (*MemoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, ok := s.memory[scope+"\x00"+key]
	if !ok || x.Deleted {
		return nil, ErrNotFound
	}
	c := *x
	return &c, nil
}

// Memory lists live items (all scopes when scope is empty), most recently updated first.
func (s *Store) Memory(scope string, limit int) []MemoryItem {
	s.mu.Lock()
	out := []MemoryItem{}
	for _, x := range s.memory {
		if !x.Deleted && (scope == "" || x.Scope == scope) {
			out = append(out, *x)
		}
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
