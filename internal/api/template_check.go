package api

import (
	"regexp"
	"strings"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
)

var (
	templateSlugRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)
	templateImageRe = regexp.MustCompile(`^[a-z0-9][a-z0-9/._-]{1,80}$`)
	templateIconRe  = regexp.MustCompile(`^[a-z0-9-]{1,20}$`)
)

// checkTemplate validates and normalises an operator-edited template. "" = ok.
func checkTemplate(t *db.Template, c config.Workspaces) string {
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
		return "Size out of range (CPU 1–" + itoa(l.MaxCPU) + ", RAM 1–" + itoa(l.MaxMemoryGiB) + " GiB, disk " + itoa(l.MinDiskGiB) + "–" + itoa(l.MaxDiskGiB) + " GiB)."
	case t.Repo != "" && hostd.ValidRepo(t.Repo, c.RepoHosts) != nil:
		return repoMsg(c)
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
	ps := make([]db.Preview, 0, len(t.Ports))
	for _, p := range t.Ports {
		ps = append(ps, db.Preview{Port: p})
	}
	if msg := checkPreviews(ps, c); msg != "" {
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
