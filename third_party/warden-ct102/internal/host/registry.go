// Registry — load and validate the host inventory from YAML.
//
// Production: /etc/warden/hosts.yaml + /etc/warden/hosts/<name>/host.yaml.
// Dev:        <repo>/registry/hosts.yaml (a sibling of this Go package, for fixtures).
//
// One owner of YAML reads. Every other package gets hosts via the registry,
// never via direct file access.

package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry is the in-memory index of managed hosts.
type Registry struct {
	mu    sync.RWMutex
	root  string         // root config directory (e.g., /etc/warden)
	hosts map[string]*Host
}

// NewRegistry constructs an empty registry rooted at the given config dir.
// Call Load() to populate it from disk.
func NewRegistry(rootDir string) *Registry {
	return &Registry{
		root:  rootDir,
		hosts: map[string]*Host{},
	}
}

// hostsIndex matches the top-level shape of /etc/warden/hosts.yaml:
//
//	version: 1
//	hosts:
//	  - name: server1
//	  - name: server2
type hostsIndex struct {
	Version int             `yaml:"version"`
	Hosts   []hostsIndexRef `yaml:"hosts"`
}

type hostsIndexRef struct {
	Name string `yaml:"name"`
}

// Load reads the hosts index and per-host detail files into memory.
// Subsequent calls overwrite the in-memory state atomically.
func (r *Registry) Load() error {
	indexPath := filepath.Join(r.root, "hosts.yaml")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No hosts.yaml = empty registry. Not an error in dev.
			r.replace(map[string]*Host{})
			return nil
		}
		return fmt.Errorf("read %s: %w", indexPath, err)
	}

	var idx hostsIndex
	if err := yaml.Unmarshal(indexBytes, &idx); err != nil {
		return fmt.Errorf("parse %s: %w", indexPath, err)
	}

	hosts := make(map[string]*Host, len(idx.Hosts))
	for _, ref := range idx.Hosts {
		if ref.Name == "" {
			return fmt.Errorf("hosts.yaml: host with empty name")
		}
		h, herr := r.loadHostDetail(ref.Name)
		if herr != nil {
			return fmt.Errorf("host %q: %w", ref.Name, herr)
		}
		if h.Name != ref.Name {
			return fmt.Errorf("host %q: index name does not match host.yaml name %q", ref.Name, h.Name)
		}
		hosts[h.Name] = h
	}

	r.replace(hosts)
	return nil
}

func (r *Registry) loadHostDetail(name string) (*Host, error) {
	path := filepath.Join(r.root, "hosts", name, "host.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var h Host
	if err := yaml.Unmarshal(body, &h); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if !h.Kind.IsKnown() {
		return nil, fmt.Errorf("unknown host kind %q", h.Kind)
	}
	if len(h.Addresses) == 0 {
		return nil, fmt.Errorf("host has no addresses")
	}
	return &h, nil
}

func (r *Registry) replace(hosts map[string]*Host) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hosts = hosts
}

// Get returns the host with the given name, or nil if not registered.
func (r *Registry) Get(name string) *Host {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hosts[name]
}

// All returns a copy of the in-memory host slice. Callers may mutate the
// returned slice; the underlying Host values are still shared with the
// registry, so do not mutate them.
func (r *Registry) All() []*Host {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Host, 0, len(r.hosts))
	for _, h := range r.hosts {
		out = append(out, h)
	}
	return out
}

// Count reports how many hosts are currently registered.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.hosts)
}
