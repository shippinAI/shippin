package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store holds one service instance's state: an org (tenant) id and three JSON-lines files.
type Store struct {
	mu        sync.Mutex
	dir, org  string
	sessions  map[string]*Session
	order     []string
	memory    map[string]*MemoryItem // scope + "\x00" + key
	decisions []Decision
}

const (
	sessionsFile  = "sessions.jsonl"
	memoryFile    = "memory.jsonl"
	decisionsFile = "decisions.jsonl"
)

// Open loads (or creates) the store in dir for org.
func Open(dir, org string) (*Store, error) {
	if org == "" {
		return nil, fmt.Errorf("org id required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, org: org, sessions: map[string]*Session{}, memory: map[string]*MemoryItem{}}
	err := readLines(filepath.Join(dir, sessionsFile), func(b []byte) error {
		var x Session
		if err := json.Unmarshal(b, &x); err != nil {
			return err
		}
		if _, seen := s.sessions[x.ID]; !seen {
			s.order = append(s.order, x.ID)
		}
		s.sessions[x.ID] = &x
		return nil
	})
	if err == nil {
		err = readLines(filepath.Join(dir, memoryFile), func(b []byte) error {
			var x MemoryItem
			if err := json.Unmarshal(b, &x); err != nil {
				return err
			}
			s.memory[x.Scope+"\x00"+x.Key] = &x
			return nil
		})
	}
	if err == nil {
		err = readLines(filepath.Join(dir, decisionsFile), func(b []byte) error {
			var x Decision
			if err := json.Unmarshal(b, &x); err != nil {
				return err
			}
			s.decisions = append(s.decisions, x)
			return nil
		})
	}
	return s, err
}

// readLines calls fn for every non-empty line; a missing file is empty. A torn last line (crash
// mid-append) is cut off so the store still opens and the next append starts on a clean line.
func readLines(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var pending error
	var good, off int64
	for sc.Scan() {
		if pending != nil {
			return pending // a bad line that wasn't the last one is real corruption
		}
		off += int64(len(sc.Bytes())) + 1
		if len(sc.Bytes()) > 0 {
			pending = fn(sc.Bytes())
		}
		if pending == nil {
			good = off
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if st, err := f.Stat(); err == nil && st.Size() > good {
		return os.Truncate(path, good)
	}
	return nil
}

// appendLine writes one JSON line and fsyncs it before the caller updates memory.
func (s *Store) appendLine(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Org is the tenant this instance belongs to.
func (s *Store) Org() string { return s.org }
