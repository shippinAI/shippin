// Package stateapi serves rrflow/state over HTTP to the workspaces bound to this instance
// (07-multi-tenancy "binding sb-<uuid>: service instance → workspace").
package stateapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"
)

// Binding is one bound workspace. Only the token's SHA-256 is kept; the token itself exists only
// in the bound workspace's home (mode 0600) and is never stored by Shippin's control plane.
type Binding struct {
	Workspace string `json:"workspace_id"`
	TokenHash string `json:"token_sha256"`
	// Namespace is the project namespace the workspace reads and writes ("" = instance root).
	Namespace string `json:"namespace,omitempty"`
}

var (
	uuidRe  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)
)

// Bindings is the binding file, reloaded when its mtime changes (hostd edits it via the CLI).
type Bindings struct {
	Path  string
	mu    sync.Mutex
	mtime time.Time
	byID  map[string]Binding
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func (b *Bindings) load() map[string]Binding {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, err := os.Stat(b.Path)
	if err != nil {
		b.byID = map[string]Binding{}
		return b.byID
	}
	if b.byID != nil && st.ModTime().Equal(b.mtime) {
		return b.byID
	}
	m := map[string]Binding{}
	if raw, err := os.ReadFile(b.Path); err == nil && json.Unmarshal(raw, &m) == nil {
		b.byID, b.mtime = m, st.ModTime()
	} else if b.byID == nil {
		b.byID = map[string]Binding{}
	}
	return b.byID
}

// Authenticate maps a bearer token to its binding (constant-time hash compare).
func (b *Bindings) Authenticate(tok string) (Binding, bool) {
	if !tokenRe.MatchString(tok) {
		return Binding{}, false
	}
	want := hashToken(tok)
	got, ok := Binding{}, false
	for _, x := range b.load() {
		if subtle.ConstantTimeCompare([]byte(x.TokenHash), []byte(want)) == 1 {
			got, ok = x, true
		}
	}
	return got, ok
}

// Set adds or replaces a binding with the token's hash and writes the file atomically (0600).
func (b *Bindings) Set(id, workspace, ns, tok string) error {
	if !uuidRe.MatchString(id) || !uuidRe.MatchString(workspace) || !tokenRe.MatchString(tok) || (ns != "" && !uuidRe.MatchString(ns)) {
		return fmt.Errorf("invalid binding")
	}
	m := copyOf(b.load())
	m[id] = Binding{Workspace: workspace, TokenHash: hashToken(tok), Namespace: ns}
	return b.write(m)
}

// Remove deletes a binding (its token stops working on the next request).
func (b *Bindings) Remove(id string) error {
	m := copyOf(b.load())
	delete(m, id)
	return b.write(m)
}

func copyOf(in map[string]Binding) map[string]Binding {
	m := make(map[string]Binding, len(in)+1)
	for k, v := range in {
		m[k] = v
	}
	return m
}

func (b *Bindings) write(m map[string]Binding) error {
	raw, _ := json.MarshalIndent(m, "", " ")
	tmp := b.Path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, b.Path); err != nil {
		return err
	}
	b.mu.Lock()
	b.byID = nil // force a reload with the new mtime
	b.mu.Unlock()
	return nil
}
