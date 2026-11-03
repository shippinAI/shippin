package hostd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var imgMu sync.Mutex

// innerImageStage is the import scratch on the inner btrfs mount. That mount is the org data
// volume (tvd-<tenant> on pool p-<tenant>). It is not the VM root disk.
const innerImageStage = "/var/lib/incus/storage-pools/" + InnerPool + "/.shippin-img/"

// ensureInnerImage makes alias inside the tenant VM point at the same image (fingerprint) as on the
// host: the host image is exported once into the state dir cache, pushed into the VM, imported there.
func (s *Server) ensureInnerImage(ctx context.Context, at place, alias string) error {
	imgMu.Lock()
	defer imgMu.Unlock()
	var a struct {
		Target string `json:"target"`
	}
	if err := query(ctx, "/1.0/images/aliases/"+alias, &a); err != nil {
		return fmt.Errorf("template image %s not found", alias)
	}
	if !fingerprintRe.MatchString(a.Target) {
		return fmt.Errorf("unexpected image fingerprint")
	}
	var in struct {
		Target string `json:"target"`
	}
	if at.query(ctx, "/1.0/images/aliases/"+alias, &in) == nil && in.Target == a.Target {
		return nil
	}
	files, err := s.exportImage(ctx, a.Target)
	if err != nil {
		return err
	}
	// Stage the import on the inner btrfs mount, which is the org data volume (tvd-*),
	// not the VM root. The files are removed after import.
	pr, vm, dir := "--project="+Project(at.t), TenantVM(at.t), innerImageStage
	args := []string{"image", "import"}
	for _, f := range files {
		if _, err := incus(ctx, "file", "push", f, vm+dir+filepath.Base(f), pr, "--create-dirs", "--mode=0600"); err != nil {
			return err
		}
		args = append(args, dir+filepath.Base(f))
	}
	defer func() {
		for _, f := range files {
			_, _ = incus(ctx, "file", "delete", vm+dir+filepath.Base(f), pr)
		}
	}()
	if !at.has(ctx, "query", "/1.0/images/"+a.Target+"?project="+at.project()) {
		if _, err := at.do(ctx, args...); err != nil {
			return err
		}
	}
	_, _ = at.do(ctx, "image", "alias", "delete", alias)
	_, err = at.do(ctx, "image", "alias", "create", alias, a.Target)
	return err
}

// exportImage exports a host image to <state_dir>/images/<fp>/ once and returns its file(s)
// (metadata tarball first for split images, as incus image import expects).
func (s *Server) exportImage(ctx context.Context, fp string) ([]string, error) {
	dir := filepath.Join(s.cfg.StateDir, "images", fp)
	if files := imageFiles(dir); len(files) > 0 {
		return files, nil
	}
	tmp := dir + ".part"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, err
	}
	if _, err := incus(ctx, "image", "export", fp, tmp+"/"+fp); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return nil, err
	}
	if files := imageFiles(dir); len(files) > 0 {
		return files, nil
	}
	return nil, fmt.Errorf("image export produced no files")
}

func imageFiles(dir string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, "*"))
	var meta, rest []string
	for _, f := range m {
		if filepath.Base(f) == filepath.Base(dir)+".tar.xz" || filepath.Base(f) == "meta-"+filepath.Base(dir)+".tar.xz" {
			meta = append(meta, f)
		} else {
			rest = append(rest, f)
		}
	}
	return append(meta, rest...)
}
