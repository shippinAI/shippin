package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// PlatformSystem is a row in the Wardenclyffe platform inventory shown on the dashboard.
type PlatformSystem struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Scope     string    `json:"scope"`
	PublicURL string    `json:"public_url,omitempty"`
	CheckHost string    `json:"check_host,omitempty"`
	CheckPort int       `json:"check_port,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type platformSystemStore struct {
	Systems []PlatformSystem `json:"systems"`
}

var (
	platformSystemsMu sync.RWMutex
	platformSystems   []PlatformSystem
	platformPath      string
	idSafeRe          = regexp.MustCompile(`[^a-z0-9-]+`)
)

func init() {
	platformPath = resolvePlatformSystemsPath()
	loadPlatformSystems()
}

func resolvePlatformSystemsPath() string {
	if d := strings.TrimSpace(os.Getenv("WARDEN_STATE_DIR")); d != "" {
		_ = os.MkdirAll(d, 0o700)
		return filepath.Join(d, "platform_systems.json")
	}
	if st, err := os.Stat("/etc/warden"); err == nil && st.IsDir() {
		return filepath.Join("/etc/warden", "platform_systems.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.TempDir()
	}
	dir := filepath.Join(configDir, "Warden")
	_ = os.MkdirAll(dir, 0o700)
	return filepath.Join(dir, "platform_systems.json")
}

func defaultPlatformSystems() []PlatformSystem {
	now := time.Now().UTC()
	return []PlatformSystem{
		{ID: "panel", Name: "Panel", Category: "panel", Scope: "internal", CheckHost: "127.0.0.1", CheckPort: 8080, Tags: []string{"control-plane"}, CreatedAt: now},
		{ID: "identity", Name: "Identity", Category: "idp", Scope: "internal", CheckHost: "127.0.0.1", CheckPort: 9000, Tags: []string{"identity"}, CreatedAt: now},
		{ID: "hypervisor", Name: "Hypervisor", Category: "hypervisor", Scope: "internal", CheckHost: "127.0.0.1", CheckPort: 8006, Tags: []string{"compute"}, CreatedAt: now},
	}
}

func loadPlatformSystems() {
	data, err := os.ReadFile(platformPath)
	if err != nil {
		if os.IsNotExist(err) {
			platformSystemsMu.Lock()
			platformSystems = defaultPlatformSystems()
			platformSystemsMu.Unlock()
			if err := savePlatformSystemsUnlocked(); err != nil {
				log.Printf("Failed to write default platform systems: %v", err)
			}
			return
		}
		log.Printf("Failed to read platform systems: %v", err)
		return
	}
	var store platformSystemStore
	if err := json.Unmarshal(data, &store); err != nil {
		log.Printf("Failed to parse platform systems: %v", err)
		return
	}
	platformSystemsMu.Lock()
	platformSystems = store.Systems
	platformSystemsMu.Unlock()
}

func savePlatformSystemsUnlocked() error {
	store := platformSystemStore{Systems: platformSystems}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(platformPath, data, 0o600)
}

func savePlatformSystems() error {
	platformSystemsMu.Lock()
	defer platformSystemsMu.Unlock()
	return savePlatformSystemsUnlocked()
}

func listPlatformSystems() []PlatformSystem {
	platformSystemsMu.RLock()
	defer platformSystemsMu.RUnlock()
	out := make([]PlatformSystem, len(platformSystems))
	copy(out, platformSystems)
	return out
}

func replacePlatformSystems(next []PlatformSystem) error {
	platformSystemsMu.Lock()
	platformSystems = append([]PlatformSystem(nil), next...)
	sort.Slice(platformSystems, func(i, j int) bool {
		return strings.ToLower(platformSystems[i].Name) < strings.ToLower(platformSystems[j].Name)
	})
	err := savePlatformSystemsUnlocked()
	platformSystemsMu.Unlock()
	return err
}

func mergePlatformSystemsExtra(incoming []PlatformSystem) error {
	platformSystemsMu.Lock()
	seen := make(map[string]bool, len(platformSystems))
	for _, p := range platformSystems {
		seen[p.ID] = true
	}
	for _, p := range incoming {
		if p.ID == "" {
			p.ID = slugifyID(p.Name)
		}
		if seen[p.ID] {
			continue
		}
		if p.CreatedAt.IsZero() {
			p.CreatedAt = time.Now().UTC()
		}
		platformSystems = append(platformSystems, p)
		seen[p.ID] = true
	}
	sort.Slice(platformSystems, func(i, j int) bool {
		return strings.ToLower(platformSystems[i].Name) < strings.ToLower(platformSystems[j].Name)
	})
	err := savePlatformSystemsUnlocked()
	platformSystemsMu.Unlock()
	return err
}

func appendPlatformSystem(ps PlatformSystem) error {
	if ps.ID == "" {
		ps.ID = slugifyID(ps.Name)
	}
	if err := validatePlatformSystem(&ps); err != nil {
		return err
	}
	platformSystemsMu.Lock()
	for _, existing := range platformSystems {
		if existing.ID == ps.ID {
			platformSystemsMu.Unlock()
			return fmt.Errorf("duplicate id %q", ps.ID)
		}
	}
	if ps.CreatedAt.IsZero() {
		ps.CreatedAt = time.Now().UTC()
	}
	platformSystems = append(platformSystems, ps)
	sort.Slice(platformSystems, func(i, j int) bool {
		return strings.ToLower(platformSystems[i].Name) < strings.ToLower(platformSystems[j].Name)
	})
	err := savePlatformSystemsUnlocked()
	platformSystemsMu.Unlock()
	return err
}

func removePlatformSystem(id string) error {
	if id == "" || id == "local" {
		return fmt.Errorf("invalid id")
	}
	platformSystemsMu.Lock()
	defer platformSystemsMu.Unlock()
	next := platformSystems[:0]
	removed := false
	for _, ps := range platformSystems {
		if ps.ID == id {
			removed = true
			continue
		}
		next = append(next, ps)
	}
	if !removed {
		return fmt.Errorf("not found")
	}
	platformSystems = next
	return savePlatformSystemsUnlocked()
}

func slugifyID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = idSafeRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = fmt.Sprintf("system-%d", time.Now().Unix())
	}
	return s
}

func validatePlatformSystem(ps *PlatformSystem) error {
	if strings.TrimSpace(ps.Name) == "" {
		return fmt.Errorf("name required")
	}
	if ps.Scope != "customer" {
		ps.Scope = "internal"
	}
	if ps.CheckHost != "" && (ps.CheckPort < 1 || ps.CheckPort > 65535) {
		return fmt.Errorf("check_port must be 1-65535 when check_host is set")
	}
	return nil
}

func tcpReachable(host string, port int, dialTimeout time.Duration) bool {
	if host == "" || port < 1 {
		return false
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func healthFromReachable(ok bool) HealthState {
	if ok {
		return HealthOK
	}
	return HealthCritical
}

func platformSystemHostNode(ps PlatformSystem) HostNode {
	st := HealthUnknown
	conns := []HostConnection{}
	if ps.PublicURL != "" {
		conns = append(conns, HostConnection{Label: "URL", Target: ps.PublicURL, State: HealthOK})
	}
	if ps.CheckHost != "" && ps.CheckPort > 0 {
		ok := tcpReachable(ps.CheckHost, ps.CheckPort, 450*time.Millisecond)
		st = healthFromReachable(ok)
		conns = append(conns, HostConnection{Label: "TCP check", Target: fmt.Sprintf("%s:%d", ps.CheckHost, ps.CheckPort), State: st})
	} else if ps.PublicURL != "" {
		st = HealthOK
	}
	tags := append([]string{}, ps.Tags...)
	tags = append(tags, "platform")
	if ps.Category != "" {
		tags = append(tags, ps.Category)
	}
	addr := ps.PublicURL
	if addr == "" && ps.Notes != "" {
		addr = ps.Notes
	}
	if addr == "" && ps.CheckHost != "" {
		addr = fmt.Sprintf("%s:%d", ps.CheckHost, ps.CheckPort)
	}
	scope := ps.Scope
	if scope != "customer" {
		scope = "internal"
	}
	return HostNode{
		ID:          ps.ID,
		Name:        ps.Name,
		Kind:        ps.categoryOrDefault(),
		Scope:       scope,
		Status:      st,
		Address:     addr,
		Tags:        tags,
		Connections: conns,
	}
}

func (ps PlatformSystem) categoryOrDefault() string {
	if strings.TrimSpace(ps.Category) != "" {
		return ps.Category
	}
	return "platform"
}
