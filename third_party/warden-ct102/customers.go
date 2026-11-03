package main

// Customer Services drill-down — first concrete consumer of the
// component system per ADR 0038 + Spec 17.
//
// Reads registry/customer-services/<slug>.services.yaml at startup +
// (eventually) the Plane A customer_service_state LIVE SELECT. For
// the scaffold, the data flow is YAML-only; live state is stubbed.
//
// Drill-down hierarchy per Spec 17 §4:
//   /customers                                       customers index (T1: all)
//   /customers/{slug}                                customer overview
//   /customers/{slug}/devstation                     devstation services
//   /customers/{slug}/devstation/{svc}               one service detail
//   /customers/{slug}/bundles                        bundles overview
//   /customers/{slug}/identity                       Authentik posture
//   /customers/{slug}/secrets                        Infisical scope
//   /customers/{slug}/observability                  Observatory slice
//   /customers/{slug}/network                        VLAN + WardenNet
//
// File budget per warden/AGENTS.md: < 8KB. Handlers in customers_handlers.go.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Customer is one row in the Customers index page.
type Customer struct {
	AccountSlug string
	DisplayName string
	BrandFront  string // primary brand-front (effing.ai / example.invalid / example.invalid / ...)
	Tier        string // T1 | T3 | ...
	ServiceCount int
	ActiveCount  int
	ExpiringCount int
	ExpiredCount int
	LastActivity string
}

// CustomerServiceInstance mirrors one row from a customer's services YAML.
// Matches the shape of registry/customer-services/<slug>.services.yaml per
// ADR 0037 + templates/customer-service/service-contract.yaml.
type CustomerServiceInstance struct {
	ServiceID    string   `yaml:"service_id"`
	TemplateRef  string   `yaml:"template_ref"`
	InstanceUUID string   `yaml:"instance_uuid"`
	Enabled      bool     `yaml:"enabled"`
	TierEligibility []string `yaml:"tier_eligibility"`
	Host         struct {
		HostID      string `yaml:"host_id"`
		RuntimeUser string `yaml:"runtime_user"`
	} `yaml:"host"`
	AuthOverride struct {
		Kind string `yaml:"kind"`
	} `yaml:"auth_override"`
	StorageOverride struct {
		InfisicalBasePath string `yaml:"infisical_base_path"`
	} `yaml:"storage_override"`
	Catalog struct {
		VisibleInCatalog bool `yaml:"visible_in_catalog"`
		SortPriority     int  `yaml:"sort_priority"`
	} `yaml:"catalog"`
	Notes string `yaml:"notes"`
}

// CustomerServicesFile is the on-disk shape of <slug>.services.yaml.
type CustomerServicesFile struct {
	SchemaVersion       int                       `yaml:"schema_version"`
	CustomerAccountSlug string                    `yaml:"customer_account_slug"`
	LastUpdated         string                    `yaml:"last_updated"`
	Notes               string                    `yaml:"notes"`
	Services            []CustomerServiceInstance `yaml:"services"`
}

// CustomerInventory caches all customer service files. Loaded on startup;
// reloaded on SIGHUP or via /api/customers/reload (future).
type CustomerInventory struct {
	mu        sync.RWMutex
	byAccount map[string]*CustomerServicesFile
	loadedFrom string
	loadError  string
}

var customers = &CustomerInventory{
	byAccount: make(map[string]*CustomerServicesFile),
}

// customerServicesDir resolves the registry directory holding the YAML files.
// Override via WARDEN_CUSTOMER_SERVICES_DIR; otherwise standard locations.
func customerServicesDir() string {
	if v := os.Getenv("WARDEN_CUSTOMER_SERVICES_DIR"); v != "" {
		return v
	}
	for _, p := range []string{
		"/etc/warden/customer-services",
		"./registry/customer-services",
		"../registry/customer-services",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// loadCustomersFromDisk walks the registry dir + populates the in-memory cache.
func loadCustomersFromDisk() {
	dir := customerServicesDir()
	customers.mu.Lock()
	defer customers.mu.Unlock()
	customers.loadedFrom = dir
	customers.loadError = ""
	customers.byAccount = make(map[string]*CustomerServicesFile)
	if dir == "" {
		customers.loadError = "WARDEN_CUSTOMER_SERVICES_DIR unset and no default path found"
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		customers.loadError = err.Error()
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".services.yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var f CustomerServicesFile
		if err := yaml.Unmarshal(data, &f); err != nil {
			continue
		}
		if f.CustomerAccountSlug == "" {
			continue
		}
		customers.byAccount[f.CustomerAccountSlug] = &f
	}
}

// listCustomers returns the index-page rows, sorted by account_slug.
// In production this would join with Plane A's customer_service_state
// LIVE SELECT to compute Active/Expiring/Expired counts. Scaffold returns
// counts from YAML (Enabled flag == active stub).
func listCustomers() []Customer {
	customers.mu.RLock()
	defer customers.mu.RUnlock()
	out := make([]Customer, 0, len(customers.byAccount))
	for slug, f := range customers.byAccount {
		c := Customer{
			AccountSlug:  slug,
			DisplayName:  strings.Title(slug),
			BrandFront:   primaryBrandFront(f),
			Tier:         "T1",
			ServiceCount: len(f.Services),
		}
		for _, s := range f.Services {
			if s.Enabled {
				c.ActiveCount++
			}
		}
		c.LastActivity = f.LastUpdated
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountSlug < out[j].AccountSlug })
	return out
}

func primaryBrandFront(_ *CustomerServicesFile) string {
	return "node.ai"
}

// getCustomer returns the parsed file for one customer.
func getCustomer(slug string) (*CustomerServicesFile, bool) {
	customers.mu.RLock()
	defer customers.mu.RUnlock()
	f, ok := customers.byAccount[slug]
	return f, ok
}

// devstationServices returns the subset filtered to devstation-bound services.
// In production this would inspect Host.HostID against managed-workspaces seed.
func devstationServices(slug string) []CustomerServiceInstance {
	f, ok := getCustomer(slug)
	if !ok {
		return nil
	}
	out := make([]CustomerServiceInstance, 0)
	for _, s := range f.Services {
		if strings.Contains(s.Host.HostID, "devstation") {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Catalog.SortPriority < out[j].Catalog.SortPriority })
	return out
}

// getCustomerService returns one specific service by slug.
func getCustomerService(slug, serviceID string) (CustomerServiceInstance, bool) {
	f, ok := getCustomer(slug)
	if !ok {
		return CustomerServiceInstance{}, false
	}
	for _, s := range f.Services {
		if s.ServiceID == serviceID {
			return s, true
		}
	}
	return CustomerServiceInstance{}, false
}
