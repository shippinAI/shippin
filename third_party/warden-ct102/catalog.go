package main

// Catalog — inventory of Wardenclyffe compose templates.
//
// Source of truth: docker-compose.yml files in the catalog directory,
// each containing an x-warden: top-level extension key. We parse that
// key from every yml and surface it as the deployable catalog.
//
// Discovery order for the catalog directory:
//   1. WARDEN_CATALOG_DIR env var (if set and the path exists)
//   2. /etc/warden/templates/compose (production install path)
//   3. ./templates/compose (binary cwd, dev mode)
//   4. ../templates/compose (running from warden/ subdir, repo dev mode)
//
// If none of those exist, the catalog is marked Available=false with a
// clear Error message. We never silently fall back to a path that
// doesn't exist.

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ----------------------------------------------------------------------------
// Types
// ----------------------------------------------------------------------------

// CatalogPort describes one port a template exposes.
type CatalogPort struct {
	Internal int    `yaml:"internal" json:"internal"`
	External int    `yaml:"external" json:"external"`
	Protocol string `yaml:"protocol" json:"protocol"`
	Public   bool   `yaml:"public" json:"public"`
	Label    string `yaml:"label" json:"label"`
	FQDNVar  string `yaml:"fqdn_var,omitempty" json:"fqdn_var,omitempty"`
}

// CatalogSecret describes a secret Warden must generate at deploy time.
type CatalogSecret struct {
	Name             string `yaml:"name" json:"name"`
	Type             string `yaml:"type" json:"type"`
	Length           int    `yaml:"length,omitempty" json:"length,omitempty"`
	Description      string `yaml:"description,omitempty" json:"description,omitempty"`
	Rotatable        bool   `yaml:"rotatable" json:"rotatable"`
	RotationStrategy string `yaml:"rotation_strategy,omitempty" json:"rotation_strategy,omitempty"`
}

// CatalogConfigurable describes a user-tunable env var.
type CatalogConfigurable struct {
	Name        string   `yaml:"name" json:"name"`
	Default     any      `yaml:"default,omitempty" json:"default,omitempty"`
	Type        string   `yaml:"type,omitempty" json:"type,omitempty"`
	Allowed     []string `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	Range       []int    `yaml:"range,omitempty" json:"range,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Sensitive   bool     `yaml:"sensitive,omitempty" json:"sensitive,omitempty"`
}

// CatalogBootstrap describes post-deploy bootstrap.
type CatalogBootstrap struct {
	InitialSetupPath string `yaml:"initial_setup_path,omitempty" json:"initial_setup_path,omitempty"`
	Description      string `yaml:"description,omitempty" json:"description,omitempty"`
	Expires          string `yaml:"expires,omitempty" json:"expires,omitempty"`
	Notes            string `yaml:"notes,omitempty" json:"notes,omitempty"`
	WardenCanOpen    bool   `yaml:"warden_can_open,omitempty" json:"warden_can_open,omitempty"`
}

// CatalogSource describes provenance.
type CatalogSource struct {
	Type          string `yaml:"type" json:"type"`
	From          string `yaml:"from,omitempty" json:"from,omitempty"`
	URL           string `yaml:"url,omitempty" json:"url,omitempty"`
	Modifications string `yaml:"modifications,omitempty" json:"modifications,omitempty"`
}

// CatalogTemplate is the full x-warden block from one compose file,
// plus Warden-side metadata populated on load.
type CatalogTemplate struct {
	Name              string                `yaml:"name" json:"name"`
	Version           string                `yaml:"version" json:"version"`
	UpstreamVersion   string                `yaml:"upstream_version,omitempty" json:"upstream_version,omitempty"`
	Category          string                `yaml:"category" json:"category"`
	Estate            string                `yaml:"estate,omitempty" json:"estate,omitempty"`
	DefaultVisibility string                `yaml:"default_visibility,omitempty" json:"default_visibility,omitempty"`
	Description       string                `yaml:"description,omitempty" json:"description,omitempty"`
	Homepage          string                `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	License           string                `yaml:"license,omitempty" json:"license,omitempty"`
	LicenseNotes      string                `yaml:"license_notes,omitempty" json:"license_notes,omitempty"`
	Source            CatalogSource         `yaml:"source,omitempty" json:"source,omitempty"`
	Ports             []CatalogPort         `yaml:"ports,omitempty" json:"ports,omitempty"`
	Health            map[string]any        `yaml:"health,omitempty" json:"health,omitempty"`
	Secrets           []CatalogSecret       `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Bootstrap         CatalogBootstrap      `yaml:"bootstrap,omitempty" json:"bootstrap,omitempty"`
	Configurable      []CatalogConfigurable `yaml:"configurable,omitempty" json:"configurable,omitempty"`
	DependsOn         map[string]any        `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Resources         map[string]any        `yaml:"resources,omitempty" json:"resources,omitempty"`
	Lifecycle         map[string]any        `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	Placement         map[string]any        `yaml:"placement,omitempty" json:"placement,omitempty"`
	Networking        map[string]any        `yaml:"networking,omitempty" json:"networking,omitempty"`
	Compliance        map[string]any        `yaml:"compliance,omitempty" json:"compliance,omitempty"`
	Backup            map[string]any        `yaml:"backup,omitempty" json:"backup,omitempty"`
	Backups           map[string]any        `yaml:"backups,omitempty" json:"backups,omitempty"`
	BackupPolicy      map[string]any        `yaml:"backup_policy,omitempty" json:"backup_policy,omitempty"`
	Recovery          map[string]any        `yaml:"recovery,omitempty" json:"recovery,omitempty"`
	RecoveryProfile   map[string]any        `yaml:"recovery_profile,omitempty" json:"recovery_profile,omitempty"`
	Upgrade           map[string]any        `yaml:"upgrade,omitempty" json:"upgrade,omitempty"`
	StorageClasses    map[string]any        `yaml:"storage_classes,omitempty" json:"storage_classes,omitempty"`

	// Warden-side metadata, populated on load (NOT from yaml file)
	FilePath  string    `yaml:"-" json:"file_path"`
	FileBytes int64     `yaml:"-" json:"file_bytes"`
	FileMTime time.Time `yaml:"-" json:"file_mtime"`
	LoadedAt  time.Time `yaml:"-" json:"loaded_at"`
	ParseErr  string    `yaml:"-" json:"parse_err,omitempty"`
}

// CatalogInventory is the full catalog state at a point in time.
type CatalogInventory struct {
	Available  bool              `json:"available"`
	Dir        string            `json:"dir"`
	DirSource  string            `json:"dir_source"` // env | prod | dev | repo-dev | none
	Error      string            `json:"error,omitempty"`
	Templates  []CatalogTemplate `json:"templates"`
	Categories []string          `json:"categories"`
	OkCount    int               `json:"ok_count"`
	ErrCount   int               `json:"err_count"`
	LoadedAt   time.Time         `json:"loaded_at"`
	LoadMillis int64             `json:"load_millis"`
}

// CatalogPageData is what handleCatalog passes to the html template.
type CatalogPageData struct {
	HeaderData
	Inventory CatalogInventory
}

// ----------------------------------------------------------------------------
// Cache
// ----------------------------------------------------------------------------

var (
	catalogCacheMu  sync.RWMutex
	catalogCache    CatalogInventory
	catalogCacheAt  time.Time
	catalogCacheTTL = 30 * time.Second
)

// ----------------------------------------------------------------------------
// Directory discovery
// ----------------------------------------------------------------------------

// resolveCatalogDir walks the discovery order and returns the first
// existing directory along with which rule matched. If none match,
// returns ("", "none").
//
// We intentionally do NOT default to /etc/warden/templates/compose when
// nothing exists — that hides bugs.
func resolveCatalogDir() (dir string, source string) {
	if d := os.Getenv("WARDEN_CATALOG_DIR"); d != "" {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d, "env"
		}
		log.Printf("Catalog dir env WARDEN_CATALOG_DIR=%q does not exist or is not a directory; falling through", d)
	}

	candidates := []struct {
		path   string
		source string
	}{
		{"/etc/warden/templates/compose", "prod"},
		{"./templates/compose", "dev"},
		{"../templates/compose", "repo-dev"},
	}

	for _, c := range candidates {
		if st, err := os.Stat(c.path); err == nil && st.IsDir() {
			return c.path, c.source
		}
	}

	return "", "none"
}

// ----------------------------------------------------------------------------
// Loading
// ----------------------------------------------------------------------------

// loadCatalog scans the catalog directory and parses every *.yml file.
// Per-file parse errors are captured in CatalogTemplate.ParseErr AND
// emitted as log lines so operators can troubleshoot from journalctl.
func loadCatalog() CatalogInventory {
	start := time.Now()

	dir, source := resolveCatalogDir()
	inv := CatalogInventory{
		Dir:       dir,
		DirSource: source,
		LoadedAt:  start,
	}

	if dir == "" {
		inv.Available = false
		inv.Error = "no catalog directory found (checked WARDEN_CATALOG_DIR, /etc/warden/templates/compose, ./templates/compose, ../templates/compose)"
		log.Printf("Catalog load failed: %s", inv.Error)
		return inv
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		inv.Available = false
		inv.Error = fmt.Sprintf("read catalog dir %q: %v", dir, err)
		log.Printf("Catalog load failed: %v", err)
		return inv
	}
	inv.Available = true

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		path := filepath.Join(dir, name)

		t, err := parseCatalogTemplate(path)
		if err != nil {
			// Capture a stub so the operator sees the broken template in the UI
			t = CatalogTemplate{
				Name:     strings.TrimSuffix(strings.TrimSuffix(name, ".yml"), ".yaml"),
				FilePath: path,
				ParseErr: err.Error(),
				LoadedAt: time.Now(),
			}
			inv.ErrCount++
			log.Printf("Catalog template parse error [%s]: %v", name, err)
		} else {
			inv.OkCount++
		}
		inv.Templates = append(inv.Templates, t)
	}

	sort.Slice(inv.Templates, func(i, j int) bool {
		if inv.Templates[i].Category != inv.Templates[j].Category {
			return inv.Templates[i].Category < inv.Templates[j].Category
		}
		return inv.Templates[i].Name < inv.Templates[j].Name
	})

	seen := map[string]bool{}
	for _, t := range inv.Templates {
		if t.Category != "" && !seen[t.Category] {
			seen[t.Category] = true
			inv.Categories = append(inv.Categories, t.Category)
		}
	}
	sort.Strings(inv.Categories)

	inv.LoadMillis = time.Since(start).Milliseconds()
	log.Printf("Catalog loaded from %s [%s]: %d ok, %d errors, %d categories in %dms",
		dir, source, inv.OkCount, inv.ErrCount, len(inv.Categories), inv.LoadMillis)

	return inv
}

// parseCatalogTemplate reads one yml and extracts the x-warden block.
func parseCatalogTemplate(path string) (CatalogTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CatalogTemplate{}, fmt.Errorf("read: %w", err)
	}
	st, statErr := os.Stat(path)

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return CatalogTemplate{}, fmt.Errorf("yaml parse: %w", err)
	}

	xwarden, ok := doc["x-warden"]
	if !ok {
		return CatalogTemplate{}, fmt.Errorf("missing x-warden top-level key")
	}

	xb, err := yaml.Marshal(xwarden)
	if err != nil {
		return CatalogTemplate{}, fmt.Errorf("re-marshal x-warden: %w", err)
	}

	var t CatalogTemplate
	if err := yaml.Unmarshal(xb, &t); err != nil {
		return CatalogTemplate{}, fmt.Errorf("decode x-warden: %w", err)
	}

	if t.Name == "" {
		return CatalogTemplate{}, fmt.Errorf("x-warden.name is required but missing")
	}
	if t.Category == "" {
		return CatalogTemplate{}, fmt.Errorf("x-warden.category is required but missing (template %q)", t.Name)
	}
	if err := validateCatalogBoundary(t); err != nil {
		return CatalogTemplate{}, err
	}

	t.Estate = estateForTemplate(t)
	t.DefaultVisibility = visibilityForTemplate(t)
	t.FilePath = path
	t.LoadedAt = time.Now()
	if statErr == nil {
		t.FileBytes = st.Size()
		t.FileMTime = st.ModTime()
	}
	return t, nil
}

func validateCatalogBoundary(t CatalogTemplate) error {
	t.Category = strings.TrimSpace(t.Category)
	if !strings.EqualFold(t.Category, "media") {
		return nil
	}
	if strings.TrimSpace(t.Estate) == "" {
		return fmt.Errorf("media template %q must declare x-warden.estate: mediastack", t.Name)
	}
	if strings.TrimSpace(t.DefaultVisibility) == "" {
		return fmt.Errorf("media template %q must declare x-warden.default_visibility: internal", t.Name)
	}
	t.Estate = normalizeEstate(t.Estate)
	t.DefaultVisibility = normalizeVisibility(t.DefaultVisibility)
	if t.Estate != EstateMediastack {
		return fmt.Errorf("media template %q must use x-warden.estate: mediastack", t.Name)
	}
	if t.DefaultVisibility != VisibilityInternal {
		return fmt.Errorf("media template %q must use x-warden.default_visibility: internal", t.Name)
	}
	if len(t.StorageClasses) == 0 {
		return fmt.Errorf("media template %q must declare x-warden.storage_classes", t.Name)
	}
	for _, p := range t.Ports {
		if p.Public {
			return fmt.Errorf("media template %q cannot expose public ports by default", t.Name)
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Cache access
// ----------------------------------------------------------------------------

// getCatalog returns the cached inventory if fresh, otherwise reloads.
func getCatalog() CatalogInventory {
	catalogCacheMu.RLock()
	if time.Since(catalogCacheAt) < catalogCacheTTL && catalogCache.Dir != "" {
		c := catalogCache
		catalogCacheMu.RUnlock()
		return c
	}
	catalogCacheMu.RUnlock()

	inv := loadCatalog()

	catalogCacheMu.Lock()
	catalogCache = inv
	catalogCacheAt = time.Now()
	catalogCacheMu.Unlock()
	return inv
}

// invalidateCatalogCache forces the next getCatalog() call to reload.
func invalidateCatalogCache() {
	catalogCacheMu.Lock()
	catalogCache = CatalogInventory{}
	catalogCacheAt = time.Time{}
	catalogCacheMu.Unlock()
	log.Printf("Catalog cache invalidated")
}
