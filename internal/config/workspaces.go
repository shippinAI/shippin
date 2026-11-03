package config

import "errors"

// Size is a create-flow size preset.
type Size struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CPU       int    `json:"cpu"`
	MemoryGiB int    `json:"memory_gib"`
	DiskGiB   int    `json:"disk_gib"`
}

// Guards are the input limits for workspaces and templates (kept in config, mirrored by hostd).
type Guards struct {
	// RepoHosts are the git hosts a workspace may clone from (public https only).
	RepoHosts []string `json:"repo_hosts"`
	// ReservedPorts can't be firewall rule ports (the edge's own 80/443 etc.).
	ReservedPorts []int `json:"reserved_ports"`
	// ReservedDomains: custom domains equal to or under these are refused.
	ReservedDomains []string       `json:"reserved_domains"`
	TemplateLimits  TemplateLimits `json:"template_limits"`
}

// TemplateLimits bound operator-edited templates.
type TemplateLimits struct {
	MaxCPU       int `json:"max_cpu"`
	MaxMemoryGiB int `json:"max_memory_gib"`
	MinDiskGiB   int `json:"min_disk_gib"`
	MaxDiskGiB   int `json:"max_disk_gib"`
	MaxPorts     int `json:"max_ports"`
	MaxStartup   int `json:"max_startup_bytes"`
}

// Check fails closed when a guard is missing (a zero value would widen what's allowed).
func (w Workspaces) Check() error {
	l := w.TemplateLimits
	switch {
	case w.PortMin < 1024 || w.PortMax < w.PortMin:
		return errors.New("workspaces.port_min/port_max must be a range above 1023")
	case w.PreviewMin < 1024 || w.PreviewMax < w.PreviewMin || w.PreviewMax > 65535:
		return errors.New("workspaces.preview_min/preview_max must be a range within 1024-65535")
	case w.ImagePrefix == "" || len(w.RepoHosts) == 0 || len(w.ReservedDomains) == 0:
		return errors.New("workspaces.image_prefix, repo_hosts and reserved_domains are required")
	case l.MaxCPU < 1 || l.MaxMemoryGiB < 1 || l.MinDiskGiB < 1 || l.MaxDiskGiB < l.MinDiskGiB || l.MaxPorts < 1 || l.MaxStartup < 1:
		return errors.New("workspaces.template_limits must all be set")
	}
	return nil
}
