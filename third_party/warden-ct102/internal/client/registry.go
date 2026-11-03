// Registry — load and validate the client (tenant) inventory from YAML.
//
// Production: /etc/warden/clients.yaml + /etc/warden/clients/<slug>/client.yaml.
// Dev:        <repo>/registry/clients.yaml.

package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry is the in-memory index of clients (tenants).
type Registry struct {
	mu      sync.RWMutex
	root    string // root config directory (e.g. /etc/warden)
	clients map[string]*Client
}

// NewRegistry constructs an empty registry rooted at the given config dir.
func NewRegistry(rootDir string) *Registry {
	return &Registry{
		root:    rootDir,
		clients: map[string]*Client{},
	}
}

type clientsIndex struct {
	Version int                 `yaml:"version"`
	Clients []clientsIndexEntry `yaml:"clients"`
}

type clientsIndexEntry struct {
	Slug string `yaml:"slug"`
}

// Load reads clients.yaml + per-client detail into memory.
func (r *Registry) Load() error {
	idxPath := filepath.Join(r.root, "clients.yaml")
	body, err := os.ReadFile(idxPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No clients file = empty registry; system tenant only.
			r.replace(map[string]*Client{})
			return nil
		}
		return fmt.Errorf("read %s: %w", idxPath, err)
	}

	var idx clientsIndex
	if err := yaml.Unmarshal(body, &idx); err != nil {
		return fmt.Errorf("parse %s: %w", idxPath, err)
	}

	clients := make(map[string]*Client, len(idx.Clients))
	for _, ref := range idx.Clients {
		if ref.Slug == "" {
			return fmt.Errorf("clients.yaml: client with empty slug")
		}
		c, cerr := r.loadClientDetail(ref.Slug)
		if cerr != nil {
			return fmt.Errorf("client %q: %w", ref.Slug, cerr)
		}
		if c.Slug != ref.Slug {
			return fmt.Errorf("client %q: index slug does not match client.yaml slug %q", ref.Slug, c.Slug)
		}
		if !c.Tier.IsKnown() {
			return fmt.Errorf("client %q: unknown tier %q", c.Slug, c.Tier)
		}
		clients[c.Slug] = c
	}

	r.replace(clients)
	return nil
}

func (r *Registry) loadClientDetail(slug string) (*Client, error) {
	path := filepath.Join(r.root, "clients", slug, "client.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Client
	if err := yaml.Unmarshal(body, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

func (r *Registry) replace(clients map[string]*Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients = clients
}

// Get returns the client with the given slug, or nil if not registered.
//
// Get("") and Get("system") return nil; the system tenant is implicit
// and is checked via Client.IsSystem() on Host.OwnerClient.
func (r *Registry) Get(slug string) *Client {
	if slug == "" || slug == "system" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[slug]
}

// All returns a copy of the registered clients.
func (r *Registry) All() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Client, 0, len(r.clients))
	for _, c := range r.clients {
		out = append(out, c)
	}
	return out
}

// Count reports how many clients are currently registered.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}
