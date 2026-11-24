package template

import (
	"regexp"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/infrastructure/hostd"
	"shippin.cloud/console/internal/platform/config"
)

var (
	templateSlugRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)
	templateImageRe = regexp.MustCompile(`^[a-z0-9][a-z0-9/._-]{1,80}$`)
	templateIconRe  = regexp.MustCompile(`^[a-z0-9-]{1,20}$`)
)

// Check validates and tidies a template staff wrote. "" = ok.
func Check(t *Template, c config.Workspaces) string {
	t.Slug, t.Name, t.Image = strings.TrimSpace(t.Slug), strings.TrimSpace(t.Name), strings.TrimSpace(t.Image)
	l := c.TemplateLimits
	if t.Icon == "" {
		t.Icon = "box"
	}
	switch {
	case !templateSlugRe.MatchString(t.Slug):
		return "Slug: lowercase letters, digits and dashes."
	case t.Name == "" || len(t.Name) > 60:
		return "Name is required (max 60 characters)."
	case !templateImageRe.MatchString(t.Image) || !strings.HasPrefix(t.Image, c.ImagePrefix):
		return "Image must be a " + c.ImagePrefix + " alias published on the host."
	case !templateIconRe.MatchString(t.Icon):
		return "Icon must be a short icon key."
	case t.CPU < 1 || t.CPU > l.MaxCPU || t.MemoryGiB < 1 || t.MemoryGiB > l.MaxMemoryGiB || t.DiskGiB < l.MinDiskGiB || t.DiskGiB > l.MaxDiskGiB:
		return "Size out of range (CPU 1–" + strconv.Itoa(l.MaxCPU) + ", RAM 1–" + strconv.Itoa(l.MaxMemoryGiB) + " GiB, disk " + strconv.Itoa(l.MinDiskGiB) + "–" + strconv.Itoa(l.MaxDiskGiB) + " GiB)."
	case t.Repo != "" && hostd.ValidRepo(t.Repo, c.RepoHosts) != nil:
		return RepoMessage(c)
	case len(t.Startup) > l.MaxStartup || len(t.Description) > 400:
		return "Startup script or description too long."
	case len(t.Ports) > l.MaxPorts || len(t.Toolchain) > 12:
		return "Too many exposed ports or toolchain entries."
	}
	for _, x := range t.Toolchain {
		if x == "" || len(x) > 30 {
			return "Toolchain entries must be 1–30 characters."
		}
	}
	if msg := CheckPorts(t.Ports, c); msg != "" {
		return "Exposed ports: " + msg
	}
	if t.Toolchain == nil {
		t.Toolchain = []string{}
	}
	if t.Ports == nil {
		t.Ports = []int{}
	}
	return ""
}

// RepoMessage says which repositories are accepted.
func RepoMessage(c config.Workspaces) string {
	return "Repository must be a public https URL on " + strings.Join(c.RepoHosts, ", ") + "."
}

// CheckPorts: each port once, inside the preview range, not the editor's port. "" = ok.
func CheckPorts(ports []int, c config.Workspaces) string {
	seen := map[int]bool{}
	for _, p := range ports {
		if p < c.PreviewMin || p > c.PreviewMax || p == c.IDEPort || seen[p] {
			return "Preview ports must be unique and between " + strconv.Itoa(c.PreviewMin) + " and " + strconv.Itoa(c.PreviewMax) + "."
		}
		seen[p] = true
	}
	return ""
}
