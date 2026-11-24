package state

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
)

// Namespaces is per-project state inside one shared instance (Notion D36: one rrflow per tenant
// VM, shared by its containers and projects). Namespace ns lives in <root>/ns/<ns>, opened on
// first use; the empty namespace is the instance root (pre-D36 per-project instances).
type Namespaces struct {
	Root, Org string
	mu        sync.Mutex
	open      map[string]*Store
}

var nsRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidNamespace reports whether ns is a usable namespace id (a Shippin project UUID, or "" for root).
func ValidNamespace(ns string) bool { return ns == "" || nsRe.MatchString(ns) }

// Dir is where namespace ns is stored.
func (n *Namespaces) Dir(ns string) (string, error) {
	if !ValidNamespace(ns) {
		return "", fmt.Errorf("%w: namespace", ErrInvalid)
	}
	if ns == "" {
		return n.Root, nil
	}
	return filepath.Join(n.Root, "ns", ns), nil
}

// Get returns namespace ns's store, opening it once.
func (n *Namespaces) Get(ns string) (*Store, error) {
	dir, err := n.Dir(ns)
	if err != nil {
		return nil, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if st, ok := n.open[ns]; ok {
		return st, nil
	}
	st, err := Open(dir, n.Org)
	if err != nil {
		return nil, err
	}
	if n.open == nil {
		n.open = map[string]*Store{}
	}
	n.open[ns] = st
	return st, nil
}
