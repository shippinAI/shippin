package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type DomainRegistryView struct {
	Available     bool                  `json:"available" yaml:"-"`
	Path          string                `json:"path" yaml:"-"`
	Error         string                `json:"error,omitempty" yaml:"-"`
	Version       int                   `json:"version" yaml:"version"`
	LastVerified  string                `json:"last_verified" yaml:"last_verified"`
	WorkspaceID   string                `json:"workspace_id" yaml:"workspace_id"`
	WorkspaceUUID string                `json:"workspace_uuid" yaml:"workspace_uuid"`
	ProjectKey    string                `json:"project_key" yaml:"project_key"`
	Owner         DomainRegistryOwner   `json:"owner" yaml:"owner"`
	PublicEdge    DomainPublicEdge      `json:"public_edge" yaml:"public_edge"`
	Providers     []DomainProvider      `json:"providers" yaml:"providers"`
	Domains       []DomainZone          `json:"domains" yaml:"domains"`
	Routes        []DomainRoute         `json:"routes" yaml:"routes"`
	Roadmap       []DomainRoadmapItem   `json:"roadmap" yaml:"roadmap"`
	Summary       DomainRegistrySummary `json:"summary" yaml:"-"`
}

type DomainRegistryOwner struct {
	Operator      string `json:"operator" yaml:"operator"`
	CustomerSlug  string `json:"customer_slug" yaml:"customer_slug"`
	Estate        string `json:"estate" yaml:"estate"`
	SourceOfTruth string `json:"source_of_truth" yaml:"source_of_truth"`
}

type DomainPublicEdge struct {
	ID             string `json:"id" yaml:"id"`
	Implementation string `json:"implementation" yaml:"implementation"`
	HostID         string `json:"host_id" yaml:"host_id"`
	HostVMID       int    `json:"host_vmid" yaml:"host_vmid"`
	InternalIP     string `json:"internal_ip" yaml:"internal_ip"`
	PublicIP       string `json:"public_ip" yaml:"public_ip"`
	ConfigPath     string `json:"config_path" yaml:"config_path"`
	Status         string `json:"status" yaml:"status"`
	Notes          string `json:"notes" yaml:"notes"`
}

type DomainProvider struct {
	ID           string   `json:"id" yaml:"id"`
	Type         string   `json:"type" yaml:"type"`
	Provider     string   `json:"provider" yaml:"provider"`
	Status       string   `json:"status" yaml:"status"`
	HostID       string   `json:"host_id" yaml:"host_id"`
	InternalIP   string   `json:"internal_ip" yaml:"internal_ip"`
	Zones        []string `json:"zones" yaml:"zones"`
	SecretRef    string   `json:"secret_ref" yaml:"secret_ref"`
	APISecretRef string   `json:"api_secret_ref" yaml:"api_secret_ref"`
}

type DomainZone struct {
	Name                string         `json:"name" yaml:"name"`
	Role                string         `json:"role" yaml:"role"`
	Status              string         `json:"status" yaml:"status"`
	Registrar           string         `json:"registrar" yaml:"registrar"`
	PublicProviderID    string         `json:"public_provider_id" yaml:"public_provider_id"`
	InternalProviderID  string         `json:"internal_provider_id" yaml:"internal_provider_id"`
	DefaultPublicTarget string         `json:"default_public_target" yaml:"default_public_target"`
	InternalZone        bool           `json:"internal_zone" yaml:"internal_zone"`
	DNSSEC              string         `json:"dnssec" yaml:"dnssec"`
	OwnerContext        string         `json:"owner_context" yaml:"owner_context"`
	Records             []DomainRecord `json:"records" yaml:"records"`
}

type DomainRecord struct {
	Host      string `json:"host" yaml:"host"`
	FQDN      string `json:"fqdn" yaml:"fqdn"`
	Type      string `json:"type" yaml:"type"`
	Target    string `json:"target" yaml:"target"`
	ProxyMode string `json:"proxy_mode" yaml:"proxy_mode"`
	Status    string `json:"status" yaml:"status"`
	RouteID   string `json:"route_id" yaml:"route_id"`
}

type DomainRoute struct {
	ID               string `json:"id" yaml:"id"`
	FQDN             string `json:"fqdn" yaml:"fqdn"`
	EdgeID           string `json:"edge_id" yaml:"edge_id"`
	BackendServiceID string `json:"backend_service_id" yaml:"backend_service_id"`
	BackendURL       string `json:"backend_url" yaml:"backend_url"`
	AuthPolicy       string `json:"auth_policy" yaml:"auth_policy"`
	Status           string `json:"status" yaml:"status"`
	Context          string `json:"context" yaml:"context"`
}

type DomainRoadmapItem struct {
	ID      string `json:"id" yaml:"id"`
	Status  string `json:"status" yaml:"status"`
	Outcome string `json:"outcome" yaml:"outcome"`
}

type DomainRegistrySummary struct {
	DomainCount       int `json:"domain_count"`
	RecordCount       int `json:"record_count"`
	RouteCount        int `json:"route_count"`
	InternalZoneCount int `json:"internal_zone_count"`
	ActiveCount       int `json:"active_count"`
	PendingCount      int `json:"pending_count"`
}

func getDomainRegistry() DomainRegistryView {
	path, checked := resolveDomainRegistryPath()
	if path == "" {
		return DomainRegistryView{
			Available: false,
			Error:     "domain registry not found; checked " + strings.Join(checked, ", "),
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return DomainRegistryView{Available: false, Path: path, Error: err.Error()}
	}

	var view DomainRegistryView
	if err := yaml.Unmarshal(data, &view); err != nil {
		return DomainRegistryView{Available: false, Path: path, Error: err.Error()}
	}
	view.Available = true
	view.Path = path
	view.normalize()
	return view
}

func resolveDomainRegistryPath() (string, []string) {
	var candidates []string
	if envPath := strings.TrimSpace(os.Getenv("WARDEN_DOMAIN_REGISTRY")); envPath != "" {
		candidates = append(candidates, envPath)
	}
	candidates = append(candidates,
		filepath.Join("registry", "domain-management", "domains.yaml"),
		filepath.Join("..", "registry", "domain-management", "domains.yaml"),
		filepath.Join("/etc", "warden", "domain-management", "domains.yaml"),
	)

	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate, candidates
			}
			return abs, candidates
		}
	}
	return "", candidates
}

func (v *DomainRegistryView) normalize() {
	sort.Slice(v.Domains, func(i, j int) bool {
		return strings.ToLower(v.Domains[i].Name) < strings.ToLower(v.Domains[j].Name)
	})
	for i := range v.Domains {
		sort.Slice(v.Domains[i].Records, func(a, b int) bool {
			return strings.ToLower(v.Domains[i].Records[a].FQDN) < strings.ToLower(v.Domains[i].Records[b].FQDN)
		})
		for j := range v.Domains[i].Records {
			rec := &v.Domains[i].Records[j]
			if rec.FQDN == "" && rec.Host != "" {
				if rec.Host == "@" {
					rec.FQDN = v.Domains[i].Name
				} else {
					rec.FQDN = fmt.Sprintf("%s.%s", rec.Host, v.Domains[i].Name)
				}
			}
		}
	}
	sort.Slice(v.Routes, func(i, j int) bool {
		return strings.ToLower(v.Routes[i].FQDN) < strings.ToLower(v.Routes[j].FQDN)
	})
	v.Summary = summarizeDomainRegistry(*v)
}

func summarizeDomainRegistry(v DomainRegistryView) DomainRegistrySummary {
	var s DomainRegistrySummary
	s.DomainCount = len(v.Domains)
	s.RouteCount = len(v.Routes)
	for _, d := range v.Domains {
		s.RecordCount += len(d.Records)
		if d.InternalZone {
			s.InternalZoneCount++
		}
		status := strings.ToLower(d.Status)
		if strings.Contains(status, "active") || strings.Contains(status, "live") {
			s.ActiveCount++
		}
		if strings.Contains(status, "pending") || strings.Contains(status, "blocked") ||
			strings.Contains(status, "confirm") || strings.Contains(status, "transfer") {
			s.PendingCount++
		}
	}
	for _, r := range v.Routes {
		status := strings.ToLower(r.Status)
		if strings.Contains(status, "pending") || strings.Contains(status, "blocked") {
			s.PendingCount++
		}
	}
	return s
}
