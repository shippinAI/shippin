package hostd

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Size overrides the plan's per-workspace defaults (0 = keep the profile default).
type Size struct {
	CPU       int `json:"cpu"`
	MemoryGiB int `json:"memory_gib"`
	DiskGiB   int `json:"disk_gib"`
}

var (
	imageRe = regexp.MustCompile(`^[a-z0-9][a-z0-9/._-]{1,80}$`)
	// repoRe: public https clone URLs only: no credentials, port, query or shell metacharacters;
	// owner and repo start with a letter/digit so the clone dir can't be "." / ".." or an option.
	repoRe = regexp.MustCompile(`^https://([a-z0-9.-]+)/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}/([A-Za-z0-9][A-Za-z0-9_.-]{0,99}?)(\.git)?/?$`)
)

// ValidRepo accepts a public https git URL on one of hosts.
func ValidRepo(url string, hosts []string) error {
	m := repoRe.FindStringSubmatch(url)
	if m == nil || !slices.Contains(hosts, m[1]) || m[2] == "" {
		return fmt.Errorf("repository must be a public https URL on %s", strings.Join(hosts, ", "))
	}
	return nil
}

func (s *Server) image(r WorkspaceReq) string {
	if r.Image == "" {
		return s.cfg.Image
	}
	return r.Image
}

func (s *Server) validTemplate(r WorkspaceReq) error {
	if r.Image != "" && (!imageRe.MatchString(r.Image) || !strings.HasPrefix(r.Image, s.cfg.Edge.ImagePrefix)) {
		return fmt.Errorf("image not allowed")
	}
	if strings.HasPrefix(r.Repo, "github:") {
		if err := ValidGitHubRepo(r.Repo); err != nil {
			return err
		}
	} else if r.Repo != "" {
		if err := ValidRepo(r.Repo, s.cfg.Edge.RepoHosts); err != nil {
			return err
		}
	}
	if r.Size.CPU < 0 || r.Size.CPU > 64 || r.Size.MemoryGiB < 0 || r.Size.MemoryGiB > 256 || r.Size.DiskGiB < 0 || r.Size.DiskGiB > 2048 {
		return fmt.Errorf("invalid size")
	}
	if len(r.Startup) > 16<<10 {
		return fmt.Errorf("startup script too long")
	}
	return s.validPreviews(r.Previews)
}

// memGiB parses an Incus-style "8GiB" value. Other units are not a size we will grow or cap.
func memGiB(s string) (int, bool) {
	n, ok := strings.CutSuffix(strings.TrimSpace(s), "GiB")
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(n)
	if err != nil || v < 1 {
		return 0, false
	}
	return v, true
}

// workspaceMemGiB parses plan workspace_memory ("8GiB"). Other units are not a cap.
func (p Plan) workspaceMemGiB() (int, bool) { return memGiB(p.WorkspaceMem) }

// capSize lowers a requested size to the plan's per-workspace CPU/memory and to the
// disk the tenant can actually hold. Zero means "profile default" and is left alone.
// A template that asks for more than the tier still provisions; it provisions at the cap.
func capSize(p Plan, z Size) Size {
	if p.WorkspaceCPU > 0 && z.CPU > p.WorkspaceCPU {
		z.CPU = p.WorkspaceCPU
	}
	if mem, ok := p.workspaceMemGiB(); ok && z.MemoryGiB > mem {
		z.MemoryGiB = mem
	}
	disk := p.QuotaGiB
	if p.VM != nil && p.VM.DataGiB > 0 && (disk == 0 || p.VM.DataGiB < disk) {
		disk = p.VM.DataGiB
	}
	if disk > 0 && z.DiskGiB > disk {
		z.DiskGiB = disk
	}
	return z
}

func sizeArgs(z Size) []string {
	a := []string{}
	if z.CPU > 0 {
		a = append(a, "-c", fmt.Sprintf("limits.cpu=%d", z.CPU))
	}
	if z.MemoryGiB > 0 {
		a = append(a, "-c", fmt.Sprintf("limits.memory=%dGiB", z.MemoryGiB))
	}
	if z.DiskGiB > 0 {
		a = append(a, "-d", fmt.Sprintf("root,size=%dGiB", z.DiskGiB))
	}
	return a
}

// bootstrap clones the repo and (re)starts the startup script on template images.
func (s *Server) bootstrap(ctx context.Context, at place, w string, r WorkspaceReq, step func(string)) error {
	imported := strings.HasPrefix(r.Repo, "github:") // GitHub imports clone on any image
	if (r.Image == "" && !imported) || (r.Repo == "" && r.Startup == "") {
		return nil
	}
	if r.Repo != "" {
		step("Cloning " + strings.TrimPrefix(r.Repo, "https://"))
	} else {
		step("Running startup script")
	}
	_, err := at.inc(ctx, r.Startup, "exec", Instance(w), "--env", "SHIPPIN_REPO="+r.Repo,
		"--", "/usr/local/sbin/shippin-ws-bootstrap")
	if err != nil {
		return fmt.Errorf("repository/startup: %w", err)
	}
	if r.Repo != "" && r.Startup != "" {
		step("Running startup script")
	}
	return nil
}

var progress sync.Map

func setProgress(w, m string) { progress.Store(w, m) }
func clearProgress(w string)  { progress.Delete(w) }

// Progress is the current provisioning step of a workspace ("" when idle).
func Progress(w string) string {
	if v, ok := progress.Load(w); ok {
		return v.(string)
	}
	return ""
}
